package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
		writeJSON(w, http.StatusOK, enrollmentResponse(p, payments.ResumeURL(p.Provider, p.ProviderPaymentID), false))
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
		switch {
		case err == nil:
			// Первая покупка: платёж только что создан.
		case errors.Is(err, repo.ErrAlreadyExists):
			// T35: providerPaymentId бесплатного платежа детерминирован,
			// поэтому повторная покупка после возврата средств снова приходит
			// сюда. CreatePayment возвращает (nil, ErrAlreadyExists) — раньше
			// это была паника на p.ID → 500. Переиспользуем тот же платёж:
			// ниже он возвращается в succeeded, доступ открывает EnsureEnrollment.
			p = freePaymentByCourse(a.Store, u.ID, c.ID)
			if p == nil {
				writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
				return
			}
		default:
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
		if errors.Is(err, payments.ErrNotConfigured) {
			// Провайдер существует, но не настроен — это сбой конфигурации, не запроса.
			writeErr(w, http.StatusBadGateway, "Платёжный сервис не настроен")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := prov.CreatePayment(r.Context(), payments.CreatePaymentRequest{
		PaymentID:   uuid.NewString(), // клиентский ключ идемпотентности у провайдера (платёж ещё не в БД)
		AmountCents: c.PriceCents,
		Currency:    c.Currency,
		Description: "Оплата курса: " + c.Title,
		ReturnURL:   payments.DefaultReturnURL(),
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

// freePaymentByCourse ищет уже выданный бесплатный платёж ученика по курсу
// (T35). Ключ детерминирован — "free-<ученик>-<курс>", поэтому при повторной
// покупке после возврата средств новый платёж не создаётся, а переиспользуется
// старый; nil означает расхождение данных (CreatePayment сказал «уже есть»,
// а платежа не нашлось) — вызывающий отвечает 500.
func freePaymentByCourse(s repo.StoreAPI, studentID, courseID string) *repo.Payment {
	want := "free-" + studentID + "-" + courseID
	for _, p := range s.ListPaymentsByStudent(studentID) {
		if p.ProviderPaymentID == want {
			return p
		}
	}
	return nil
}

// PaymentWebhook — POST /payments/webhook/{provider}: проверка подписи и
// идемпотентная обработка события. Подпись проверяется ДО разбора тела;
// повторный webhook по тому же providerPaymentID возвращает duplicate.
func (a *App) PaymentWebhook(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	prov, err := payments.NewProvider(providerName)
	if err != nil {
		if errors.Is(err, payments.ErrNotConfigured) {
			writeErr(w, http.StatusBadGateway, "Платёжный сервис не настроен")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Критически важно: подпись/источник проверяются до применения события,
	// иначе кто угодно может получить доступ к платному курсу.
	if err := prov.VerifyWebhook(r, nil); err != nil {
		writeWebhookErr(w, err)
		return
	}
	event, err := prov.HandleWebhook(r)
	if err != nil {
		writeWebhookErr(w, err)
		return
	}
	p, found := a.Store.GetPaymentByProviderID(event.ProviderPaymentID)
	if !found {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	switch event.Status {
	case "succeeded":
		if p.Status == repo.PaymentSucceeded {
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
			return
		}
		a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded)
		a.Store.EnsureEnrollment(p.StudentID, p.CourseID)
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	case "failed":
		if p.Status == repo.PaymentFailed {
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
			return
		}
		a.Store.SetPaymentStatus(p.ID, repo.PaymentFailed)
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	case "refunded":
		if p.Status == repo.PaymentRefunded {
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
			return
		}
		if p.Status != repo.PaymentSucceeded {
			// Возвращать можно только оплаченный платёж.
			writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
			return
		}
		a.Store.SetPaymentStatus(p.ID, repo.PaymentRefunded)
		// Возврат средств отменяет доступ: запись на курс становится refunded.
		// Ошибка намеренно игнорируется: платёж уже переведён в refunded,
		// повторный webhook вернёт duplicate и не дойдёт сюда.
		_ = a.Store.SetEnrollmentStatus(p.StudentID, p.CourseID, repo.EnrollmentRefunded)
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
	}
}

// writeWebhookErr мапит ошибки проверки подписи/провайдера на HTTP-коды:
// 401 — неверная подпись, 502 — внешний платёжный сервис недоступен, 400 — прочее.
func writeWebhookErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrUnauthorized):
		writeErr(w, http.StatusUnauthorized, "Недействительная подпись вебхука")
	case errors.Is(err, payments.ErrUpstream):
		writeErr(w, http.StatusBadGateway, "Платёжный сервис недоступен: "+err.Error())
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}
