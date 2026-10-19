package handler

// T63 — промокоды: создание/список/деактивация у преподавателя, проверка
// кода на странице курса и покупка со скидкой.
//
// Источник правды — Task/00-api-contract.md, раздел «T63 — промокоды»
// (заморожен). Маршруты учителя регистрируются RegisterTeacherPromoRoutes
// внутри r.Route("/teacher", ...) рядом с остальными Register*-методами
// (middleware.go): requireAuth + requireRole(teacher, admin) уже навешены
// родителем, владение курсом проверяет ownCourse (403 — чужой, 404 — нет
// курса). Публичная проверка кода POST /courses/{id}/promo/validate
// регистрируется отдельно с requireAuth — «любой вошедший».
//
// Тексты ошибок дословно совпадают с контрактом: их проверяют контрактные
// тесты и клиент показывает их пользователю как есть.

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// promoCodeRe — допустимый код после trim+upper (контракт T63):
// латиница, цифры, дефис или подчёркивание, 3–32 символа.
var promoCodeRe = regexp.MustCompile(`^[A-Z0-9_-]{3,32}$`)

// Тексты ошибок T63 (замороженный контракт, дословно).
const (
	promoCodeInvalidErr  = "Код: 3–32 символа — латиница, цифры, дефис или подчёркивание"
	promoDiscountErr     = "Скидка должна быть целым числом от 1 до 90"
	promoMaxUsesErr      = "Лимит использований не может быть отрицательным"
	promoExpiresErr      = "Срок действия должен быть в будущем"
	promoDuplicateErr    = "Такой промокод уже существует на этом курсе"
	promoArchivedErr     = "Курс в архиве: промокоды недоступны"
	promoNotFoundErr     = "Промокод не найден"
	promoDisabledErr     = "Промокод отключён"
	promoExpiredErr      = "Промокод истёк"
	promoLimitErr        = "Лимит использований промокода исчерпан"
	promoFreeCourseErr   = "Курс бесплатный — промокод не нужен"
	promoCodeRequiredErr = "Укажите промокод"
)

// RegisterTeacherPromoRoutes вешает маршруты промокодов на /teacher-роутер
// (requireAuth + requireRole(teacher, admin) уже навешены родителем).
func (a *App) RegisterTeacherPromoRoutes(r chi.Router) {
	r.Post("/courses/{id}/promo-codes", a.TeacherCreatePromoCode)
	r.Get("/courses/{id}/promo-codes", a.TeacherListPromoCodes)
	r.Delete("/courses/{id}/promo-codes/{promoId}", a.TeacherDeactivatePromoCode)
}

// createPromoCodeRequest — тело POST /teacher/courses/{id}/promo-codes.
// maxUses и expiresAt необязательны: по умолчанию 0 (без лимита) и null
// (без срока действия).
type createPromoCodeRequest struct {
	Code        string  `json:"code"`
	DiscountPct int     `json:"discountPct"`
	MaxUses     int     `json:"maxUses"`
	ExpiresAt   *string `json:"expiresAt"`
}

// TeacherCreatePromoCode — POST /teacher/courses/{id}/promo-codes.
// Успех 201 {"promoCode": <PromoCode>}, код в ответе — в верхнем регистре.
// Архивный курс — 409 (состояние ресурса важнее валидации тела, как в
// PATCH карточки и обложке); чужой курс — 403; дубликат кода — 400.
func (a *App) TeacherCreatePromoCode(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if c.Status == repo.CourseArchived {
		writeErr(w, http.StatusConflict, promoArchivedErr)
		return
	}
	var req createPromoCodeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if !promoCodeRe.MatchString(code) {
		writeErr(w, http.StatusBadRequest, promoCodeInvalidErr)
		return
	}
	if req.DiscountPct < 1 || req.DiscountPct > 90 {
		writeErr(w, http.StatusBadRequest, promoDiscountErr)
		return
	}
	if req.MaxUses < 0 {
		writeErr(w, http.StatusBadRequest, promoMaxUsesErr)
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt))
		if err != nil || !t.After(time.Now().UTC()) {
			writeErr(w, http.StatusBadRequest, promoExpiresErr)
			return
		}
		utc := t.UTC()
		expiresAt = &utc
	}
	// Предварительная проверка дубликата: отдельный текст ошибки вместо
	// конфликта уникальности из стора (гонку закрывает ErrAlreadyExists ниже).
	if _, exists := a.Store.FindPromoCode(c.ID, code); exists {
		writeErr(w, http.StatusBadRequest, promoDuplicateErr)
		return
	}
	p, err := a.Store.CreatePromoCode(&repo.PromoCode{
		CourseID:    c.ID,
		Code:        code,
		DiscountPct: req.DiscountPct,
		MaxUses:     req.MaxUses,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		if errors.Is(err, repo.ErrAlreadyExists) {
			writeErr(w, http.StatusBadRequest, promoDuplicateErr)
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось создать промокод")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"promoCode": p})
}

// TeacherListPromoCodes — GET /teacher/courses/{id}/promo-codes:
// 200 {"items":[…]} по createdAt убыванию. Неактивные коды остаются в
// списке (история), архивный курс читать можно — контракт ограничивает
// только создание.
func (a *App) TeacherListPromoCodes(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	items := a.Store.ListPromoCodesByCourse(c.ID)
	if items == nil {
		items = []*repo.PromoCode{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// TeacherDeactivatePromoCode — DELETE /teacher/courses/{id}/promo-codes/{promoId}:
// active=false, запись сохраняется. 204; повторный вызов — тоже 204
// (идемпотентно). 404 «Промокод не найден» — в т.ч. если код принадлежит
// другому курсу (не раскрываем существование чужого кода).
func (a *App) TeacherDeactivatePromoCode(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	p, found := a.Store.GetPromoCodeByID(strings.TrimSpace(chi.URLParam(r, "promoId")))
	if !found || p.CourseID != c.ID {
		writeErr(w, http.StatusNotFound, promoNotFoundErr)
		return
	}
	if err := a.Store.DeactivatePromoCode(p.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, promoNotFoundErr)
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось отключить промокод")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validatePromoRequest — тело POST /courses/{id}/promo/validate.
type validatePromoRequest struct {
	Code string `json:"code"`
}

// ValidatePromoCode — POST /courses/{id}/promo/validate (только вошедший):
// 200 {code, discountPct, priceCents, finalPriceCents}. Скидка считается
// к целым копейкам вниз, в пользу студента:
// finalPriceCents = priceCents − ⌊priceCents·discountPct/100⌋.
// Ошибки применения кода — 400 с текстами контракта; неизвестный курс — 404.
func (a *App) ValidatePromoCode(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	var req validatePromoRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		writeErr(w, http.StatusBadRequest, promoCodeRequiredErr)
		return
	}
	p, status, msg := a.resolvePromo(c, code)
	if p == nil {
		writeErr(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": p.Code, "discountPct": p.DiscountPct,
		"priceCents": c.PriceCents, "finalPriceCents": promoFinalPrice(c.PriceCents, p.DiscountPct),
	})
}

// resolvePromo проверяет применимость кода к курсу и возвращает промокод.
// При ошибке p == nil, status — HTTP-код, msg — текст контракта. Один и тот
// же расчёт используют validate и POST /enrollments — тексты ошибок и
// порядок проверок обязаны совпадать.
//
// Порядок: бесплатный курс (код не нужен) → наличие кода → active →
// срок действия → лимит. Пустой код сюда не передаётся: вызывающий
// отвечает «Укажите промокод» / пропускает проверку (enrollment без кода).
func (a *App) resolvePromo(c *repo.Course, code string) (*repo.PromoCode, int, string) {
	if c.PriceCents <= 0 {
		return nil, http.StatusBadRequest, promoFreeCourseErr
	}
	p, found := a.Store.FindPromoCode(c.ID, code)
	if !found {
		return nil, http.StatusBadRequest, promoNotFoundErr
	}
	if !p.Active {
		return nil, http.StatusBadRequest, promoDisabledErr
	}
	if p.ExpiresAt != nil && !p.ExpiresAt.After(time.Now().UTC()) {
		return nil, http.StatusBadRequest, promoExpiredErr
	}
	if p.MaxUses > 0 && p.UsedCount >= p.MaxUses {
		return nil, http.StatusBadRequest, promoLimitErr
	}
	return p, 0, ""
}

// promoFinalPrice — цена со скидкой: процент округляется вниз до копеек
// (в пользу студента). Скидка 20% от 10000 = 2000 → 8000.
func promoFinalPrice(priceCents, discountPct int) int {
	return priceCents - priceCents*discountPct/100
}
