package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/payments"
	"edu-platform/backend/internal/repo"
	"edu-platform/backend/internal/service"
)

type enrollRequest struct {
	CourseID string `json:"courseId"`
	Provider string `json:"provider"`
}

// CreateEnrollment — POST /enrollments: создаёт платёж и ссылку на оплату.
// 409 — курс уже куплен. Бесплатный курс (0 ₽) активируется сразу без провайдера.
func (a *App) CreateEnrollment(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	var req enrollRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(req.CourseID))
	if !found || c.Status != repo.CoursePublished {
		writeErr(w, http.StatusNotFound, "Курс не найден или недоступен для покупки")
		return
	}
	if e, exists := a.Store.GetEnrollment(u.ID, c.ID); exists && e.Status == repo.EnrollmentActive {
		writeErr(w, http.StatusConflict, "Курс уже куплен, он доступен в разделе «Мои курсы»")
		return
	}
	// Повторный запрос при pending-платеже — вернуть ту же ссылку (идемпотентность).
	if p, pending := a.Store.FindPendingPayment(u.ID, c.ID); pending {
		writeJSON(w, http.StatusOK, enrollmentResponse(p, p.ProviderPaymentID, false))
		return
	}

	commission := a.Store.GetCommissionPct()
	fee, teacherAmount := service.Split(c.PriceCents, commission)
	providerName := strings.ToLower(strings.TrimSpace(req.Provider))
	if providerName == "" {
		providerName = a.Config.PaymentsProvider
	}

	// Бесплатный курс — сразу успех без провайдера.
	if c.PriceCents == 0 {
		p, err := a.Store.CreatePayment(&repo.Payment{
			StudentID: u.ID, CourseID: c.ID,
			AmountCents: 0, PlatformFeeCents: 0, TeacherAmountCents: 0,
			Currency: c.Currency, Provider: "mock",
			ProviderPaymentID: "free-" + u.ID + "-" + c.ID,
			Status:            repo.PaymentSucceeded,
		})
		if err != nil && !errors.Is(err, repo.ErrAlreadyExists) {
			writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
			return
		}
		a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded)
		a.Store.EnsureEnrollment(u.ID, c.ID)
		writeJSON(w, http.StatusCreated, enrollmentResponse(p, "", true))
		return
	}

	prov, err := payments.NewProvider(providerName)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := prov.CreatePayment(r.Context(), payments.CreatePaymentRequest{
		AmountCents: c.PriceCents,
		Currency:    c.Currency,
		Description: "Оплата курса: " + c.Title,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Платёжный сервис недоступен: "+err.Error())
		return
	}
	p, err := a.Store.CreatePayment(&repo.Payment{
		StudentID: u.ID, CourseID: c.ID,
		AmountCents: c.PriceCents, PlatformFeeCents: fee, TeacherAmountCents: teacherAmount,
		Currency: c.Currency, Provider: prov.Name(),
		ProviderPaymentID: res.ProviderPaymentID,
		Status:            repo.PaymentPending,
	})
	if err != nil {
		writeErr(w, http.StatusConflict, "Такой платёж уже создан")
		return
	}
	writeJSON(w, http.StatusCreated, enrollmentResponse(p, res.RedirectURL, false))
}

func enrollmentResponse(p *repo.Payment, redirectURL string, active bool) map[string]any {
	return map[string]any{
		"paymentId": p.ID, "providerPaymentId": p.ProviderPaymentID,
		"redirectUrl": redirectURL, "amountCents": p.AmountCents,
		"status": p.Status, "enrollmentActive": active,
	}
}

// PaymentWebhook — POST /payments/webhook/{provider}: идемпотентная обработка.
// Повторный webhook по тому же providerPaymentID возвращает duplicate.
func (a *App) PaymentWebhook(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	prov, err := payments.NewProvider(providerName)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	event, err := prov.HandleWebhook(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, found := a.Store.GetPaymentByProviderID(event.ProviderPaymentID)
	if !found {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	if p.Status == repo.PaymentSucceeded {
		writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
		return
	}
	switch event.Status {
	case "succeeded":
		a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded)
		a.Store.EnsureEnrollment(p.StudentID, p.CourseID)
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	case "failed":
		a.Store.SetPaymentStatus(p.ID, repo.PaymentFailed)
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
	}
}
