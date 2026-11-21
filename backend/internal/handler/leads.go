package handler

// Заявки на консультацию с главной страницы («Поможем подобрать курс»):
//   POST /leads                    — публично, лимит запросов на IP;
//   GET  /admin/leads              — список для администратора;
//   POST /admin/leads/{id}/status  — отметить заявку обработанной/новой.
//
// До появления этого эндпоинта форма только показывала «Заявка отправлена»,
// а контакты посетителей никуда не сохранялись.

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/mailer"
	"edu-platform/backend/internal/repo"
)

// RegisterLeadsRoutes вешает маршруты заявок на /api/v1 (пути абсолютные).
func (a *App) RegisterLeadsRoutes(r chi.Router) {
	r.With(a.rateLimit("leads", a.Config.RateLimitAuthPerMin)).Post("/leads", a.CreateLead)
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).Get("/admin/leads", a.AdminListLeads)
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).Post("/admin/leads/{id}/status", a.AdminSetLeadStatus)
}

type createLeadRequest struct {
	Name          string `json:"name"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	ContactMethod string `json:"contactMethod"`
	Consent       bool   `json:"consent"`
	// Website — поле-ловушка: скрыто от людей, боты его заполняют.
	Website string `json:"website"`
}

var leadContactMethods = map[string]bool{"phone": true, "telegram": true, "vk": true, "max": true}

// normalizePhone оставляет в номере цифры и ведущий «+»; ok=false — номер
// явно некорректен (меньше 7 или больше 15 цифр, посторонние символы).
func normalizePhone(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	var b strings.Builder
	digits := 0
	for i, r := range raw {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
			digits++
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')':
			// разделители форматирования отбрасываем
		default:
			return "", false
		}
	}
	return b.String(), digits >= 7 && digits <= 15
}

// CreateLead — POST /leads: 201 {"ok":true}; 400 с понятной ошибкой.
func (a *App) CreateLead(w http.ResponseWriter, r *http.Request) {
	var req createLeadRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Website) != "" {
		// Бот заполнил скрытое поле: отвечаем как обычно, но не сохраняем.
		writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
		return
	}
	name := strings.Join(strings.Fields(req.Name), " ")
	if n := utf8.RuneCountInString(name); n < 2 || n > 100 {
		writeErr(w, http.StatusBadRequest, "Укажите имя (от 2 до 100 символов)")
		return
	}
	phone, ok := normalizePhone(req.Phone)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Укажите корректный номер телефона")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email != "" {
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
			writeErr(w, http.StatusBadRequest, "Некорректный email")
			return
		}
	}
	method := strings.TrimSpace(req.ContactMethod)
	if method == "" {
		method = "phone"
	}
	if !leadContactMethods[method] {
		writeErr(w, http.StatusBadRequest, "Некорректный способ связи")
		return
	}
	if !req.Consent {
		writeErr(w, http.StatusBadRequest, "Нужно согласие на обработку персональных данных")
		return
	}
	lead, err := a.Store.CreateLead(repo.Lead{
		Name:          name,
		Phone:         phone,
		Email:         email,
		ContactMethod: method,
		IPAddress:     clientIP(r),
	})
	if err != nil {
		log.Printf("create lead: %v", err)
		writeErr(w, http.StatusInternalServerError, "Не удалось отправить заявку, попробуйте позже")
		return
	}
	a.notifyLead(lead)
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// notifyLead пишет администратору о новой заявке, если настроена почта.
// Без SMTP заявка всё равно сохранена и видна в админке.
func (a *App) notifyLead(l *repo.Lead) {
	to := strings.TrimSpace(a.Config.AdminEmail)
	if a.Mailer == nil || to == "" {
		return
	}
	text := fmt.Sprintf("Новая заявка на консультацию\n\nИмя: %s\nТелефон: %s\nEmail: %s\nСпособ связи: %s\nВремя: %s\n\nВсе заявки — в админке: %s/admin",
		l.Name, l.Phone, l.Email, l.ContactMethod, l.CreatedAt.Format("02.01.2006 15:04 MST"), strings.TrimRight(a.Config.PublicAppURL, "/"))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Mailer.Send(ctx, mailer.Message{To: to, Subject: "Новая заявка на консультацию", Text: text}); err != nil {
			log.Printf("lead notification failed (lead=%s): %v", l.ID, err)
		}
	}()
}

// AdminListLeads — GET /admin/leads: {"items":[Lead],"newCount":N}.
func (a *App) AdminListLeads(w http.ResponseWriter, r *http.Request) {
	items := a.Store.ListLeads(500)
	newCount := 0
	for _, l := range items {
		if l.Status == repo.LeadStatusNew {
			newCount++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "newCount": newCount})
}

// AdminSetLeadStatus — POST /admin/leads/{id}/status {"status":"new"|"done"}: 204.
func (a *App) AdminSetLeadStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Status != repo.LeadStatusNew && req.Status != repo.LeadStatusDone {
		writeErr(w, http.StatusBadRequest, "Некорректный статус заявки")
		return
	}
	if err := a.Store.SetLeadStatus(chi.URLParam(r, "id"), req.Status); err != nil {
		if err == repo.ErrNotFound {
			writeErr(w, http.StatusNotFound, "Заявка не найдена")
			return
		}
		log.Printf("set lead status: %v", err)
		writeErr(w, http.StatusInternalServerError, "Не удалось обновить заявку")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
