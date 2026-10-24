package handler

// T89 — глобальный журнал промокодов супер-админа: GET /admin/promo-codes
// (поиск, фильтры по курсу/статусу, пагинация) и PATCH
// /admin/promo-codes/{id} (частичная правка скидки/лимита/срока/активности).
//
// Контракт заморожен в Task/00-api-contract.md, раздел «T89 — глобальное
// управление промокодами». Маршруты регистрируются внутри существующей группы
// r.Route("/admin", ...) (middleware.go): requireAuth + requireRole(admin)
// уже навешены родителем; adminOnly в хендлерах — страховка, как у остальных
// admin-эндпоинтов.
//
// Валидация PATCH: неизвестное поле отсекает decodeJSON (DisallowUnknownFields
// → 400), пустое тело/`{}` — 400, discountPct 1..90, maxUses >= 0 и не меньше
// already usedCount, expiresAt — RFC3339 в будущем или null (очистка).
// Архивный курс менять нельзя — 409 (состояние ресурса важнее валидации тела,
// поэтому guard стоит до чтения тела, как в PATCH карточки курса).

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// adminPromoFieldsRequiredErr — тело PATCH без единого поля.
const adminPromoFieldsRequiredErr = "Укажите хотя бы одно поле для обновления"

// adminPromoMaxUsesBelowUsedErr — лимит ниже уже использованного количества.
const adminPromoMaxUsesBelowUsedErr = "Лимит использований не может быть меньше использованного количества"

// RegisterAdminPromosRoutes вешает маршруты журнала промокодов на /admin-роутер
// (пути относительные; requireAuth/requireRole(admin) уже навешены родителем).
func (a *App) RegisterAdminPromosRoutes(r chi.Router) {
	r.Get("/promo-codes", a.AdminListPromoCodes)
	r.Patch("/promo-codes/{id}", a.AdminUpdatePromoCode)
}

// adminPromoListResponse — ответ GET /admin/promo-codes по контракту T89.
type adminPromoListResponse struct {
	Items []*repo.AdminPromoRow `json:"items"`
	Total int                   `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
}

// AdminListPromoCodes — GET /admin/promo-codes?courseId=&q=&active=&page=&limit=:
// журнал всех промокодов платформы. courseId — UUID (иначе 400), q — подстрока
// без учёта регистра по коду или названию курса, active — true|false.
// Пагинация: page с 1, limit 20 по умолчанию, максимум 100; total — до
// пагинации. items — всегда массив (не null).
func (a *App) AdminListPromoCodes(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}
	query := r.URL.Query()
	courseID := strings.TrimSpace(query.Get("courseId"))
	if courseID != "" {
		if _, err := uuid.Parse(courseID); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр courseId должен быть UUID")
			return
		}
	}
	var active *bool
	switch strings.TrimSpace(query.Get("active")) {
	case "":
		// Фильтра нет.
	case "true":
		v := true
		active = &v
	case "false":
		v := false
		active = &v
	default:
		writeErr(w, http.StatusBadRequest, "Параметр active должен быть true или false")
		return
	}

	items, total := a.Store.ListAdminPromoCodes(repo.AdminPromoFilter{
		CourseID: courseID,
		Q:        strings.TrimSpace(query.Get("q")),
		Active:   active,
		Page:     page,
		Limit:    limit,
	})
	if items == nil {
		items = []*repo.AdminPromoRow{}
	}
	writeJSON(w, http.StatusOK, adminPromoListResponse{
		Items: items, Total: total, Page: page, Limit: limit,
	})
}

// updateAdminPromoRequest — тело PATCH /admin/promo-codes/{id}. ExpiresAt
// принимается сырым JSON, чтобы отличить отсутствие поля (не менять) от
// явного null (очистить срок); остальные поля — обычные указатели.
type updateAdminPromoRequest struct {
	DiscountPct *int            `json:"discountPct"`
	MaxUses     *int            `json:"maxUses"`
	ExpiresAt   json.RawMessage `json:"expiresAt"`
	Active      *bool           `json:"active"`
}

// AdminUpdatePromoCode — PATCH /admin/promo-codes/{id}: частичное обновление.
// Меняются только присланные поля; expiresAt=null очищает срок. Код ответа:
// 400 — пустое тело/неизвестное поле/неверные значения, 404 — промокода нет,
// 409 — курс в архиве, 200 — {"promoCode": <строка журнала>}.
func (a *App) AdminUpdatePromoCode(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	p, found := a.Store.GetPromoCodeByID(id)
	if !found {
		writeErr(w, http.StatusNotFound, promoNotFoundErr)
		return
	}
	// Архивный курс заморожен: 409 до валидации тела (даже пустое тело).
	if c, ok := a.Store.GetCourseByID(p.CourseID); ok && c.Status == repo.CourseArchived {
		writeErr(w, http.StatusConflict, promoArchivedErr)
		return
	}

	var req updateAdminPromoRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DiscountPct == nil && req.MaxUses == nil && req.ExpiresAt == nil && req.Active == nil {
		writeErr(w, http.StatusBadRequest, adminPromoFieldsRequiredErr)
		return
	}
	if req.DiscountPct != nil && (*req.DiscountPct < 1 || *req.DiscountPct > 90) {
		writeErr(w, http.StatusBadRequest, promoDiscountErr)
		return
	}
	if req.MaxUses != nil {
		if *req.MaxUses < 0 {
			writeErr(w, http.StatusBadRequest, promoMaxUsesErr)
			return
		}
		// Лимит не может стать меньше уже потраченного: код окажется
		// «переиспользованным» и не пройдёт проверку при покупке.
		if *req.MaxUses < p.UsedCount {
			writeErr(w, http.StatusBadRequest, adminPromoMaxUsesBelowUsedErr)
			return
		}
	}

	patch := repo.AdminPromoPatch{
		DiscountPct: req.DiscountPct, MaxUses: req.MaxUses, Active: req.Active,
	}
	if req.ExpiresAt != nil {
		// Указатель на nil означает «очистить»; указатель на время — новый срок.
		var expires *time.Time
		if raw := strings.TrimSpace(string(req.ExpiresAt)); raw != "null" {
			var value string
			if err := json.Unmarshal(req.ExpiresAt, &value); err != nil {
				writeErr(w, http.StatusBadRequest, promoExpiresErr)
				return
			}
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
			if err != nil || !t.After(time.Now().UTC()) {
				writeErr(w, http.StatusBadRequest, promoExpiresErr)
				return
			}
			utc := t.UTC()
			expires = &utc
		}
		patch.ExpiresAt = &expires
	}

	row, err := a.Store.UpdateAdminPromoCode(id, patch)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, promoNotFoundErr)
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось обновить промокод")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"promoCode": row})
}
