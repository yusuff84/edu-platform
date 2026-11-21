package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/payments"
	"edu-platform/backend/internal/repo"
	"edu-platform/backend/internal/service"
)

type enrollRequest struct {
	CourseID string `json:"courseId"`
	// Provider оставлен только для обратной совместимости JSON-контракта:
	// значение игнорируется, провайдер всегда выбирается на сервере по
	// Config.PaymentsProvider. Клиент не должен влиять на платёжный контур.
	Provider string `json:"provider"`
	// T63: необязательный промокод. Пустая строка (или отсутствие поля) —
	// обычная покупка без скидки.
	PromoCode string `json:"promoCode"`
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
	unlock, locked := a.lockBilling(w, r)
	if !locked {
		return
	}
	defer unlock()
	c, found := a.Store.GetCourseByID(strings.TrimSpace(req.CourseID))
	if !found || c.Status != repo.CoursePublished {
		writeErr(w, http.StatusNotFound, "Курс не найден или недоступен для покупки")
		return
	}
	if e, exists := a.Store.GetEnrollment(u.ID, c.ID); exists && e.Status == repo.EnrollmentActive {
		writeErr(w, http.StatusConflict, "Курс уже куплен, он доступен в разделе «Мои курсы»")
		return
	}

	// Провайдера выбирает только сервер. Клиентский req.Provider игнорируется:
	// иначе при PAYMENTS_PROVIDER=stripe любой студент мог бы прислать
	// {"provider":"mock"} и активировать курс через публичный mock-вебхук.
	// Пустой PaymentsProvider (тесты/локальный конструктор) означает mock —
	// та же семантика, что у payments.NewProvider.
	providerName := strings.ToLower(strings.TrimSpace(a.Config.PaymentsProvider))
	if providerName == "" {
		providerName = "mock"
	}
	if c.PriceCents > 0 && providerName == "disabled" {
		writeErr(w, http.StatusServiceUnavailable, "Платные покупки временно отключены")
		return
	}
	// В production mock запрещён полностью: его страница подтверждения не
	// умеет подписывать вебхук, поэтому платёж не создаём (503 — сбой
	// конфигурации). Бесплатные курсы провайдера не используют и остаются
	// доступны.
	if c.PriceCents > 0 && providerName == "mock" && a.mockPaymentsDisabled() {
		writeErr(w, http.StatusServiceUnavailable, mockPaymentsDisabledMsg)
		return
	}

	// Повторный запрос при pending-платеже — вернуть ту же ссылку
	// (идемпотентность). Но только если платёж принадлежит текущему
	// провайдеру: после смены PAYMENTS_PROVIDER старая ссылка нерабочая,
	// а вебхук от неё этот API всё равно отклонит.
	if p, pending := a.Store.FindPendingPayment(u.ID, c.ID); pending {
		if strings.EqualFold(strings.TrimSpace(p.Provider), providerName) {
			a.checkoutPayment(w, r, p, http.StatusOK)
			return
		}
		a.Store.SetPaymentStatus(p.ID, repo.PaymentFailed)
	}

	// T63: промокод проверяется до создания платежа — те же проверки и
	// тексты ошибок, что у POST /courses/{id}/promo/validate. Бесплатный
	// курс + код — 400 «Курс бесплатный — промокод не нужен» (resolvePromo).
	// amountCents — фактическая сумма к оплате, discountCents — разница
	// с полной ценой; сплит комиссии считается от фактической суммы.
	var promo *repo.PromoCode
	if code := strings.ToUpper(strings.TrimSpace(req.PromoCode)); code != "" {
		p, status, msg := a.resolvePromo(c, code)
		if p == nil {
			writeErr(w, status, msg)
			return
		}
		promo = p
	}
	amountCents := c.PriceCents
	discountCents := 0
	promoCode := ""
	if promo != nil {
		promoCode = promo.Code
		amountCents = promoFinalPrice(c.PriceCents, promo.DiscountPct)
		discountCents = c.PriceCents - amountCents
	}

	commission := a.Store.GetCommissionPct()
	fee, teacherAmount := service.Split(amountCents, commission)

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
		if !a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded) {
			writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
			return
		}
		if e, _ := a.Store.EnsureEnrollment(u.ID, c.ID); e == nil {
			writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
			return
		}
		writeJSON(w, http.StatusCreated, enrollmentResponse(p, "", true))
		return
	}

	if _, err := payments.NewProvider(providerName); err != nil {
		writeErr(w, http.StatusBadGateway, "Платёжный сервис не настроен")
		return
	}
	p, err := a.Store.CreatePayment(&repo.Payment{
		StudentID: u.ID, CourseID: c.ID,
		AmountCents: amountCents, PlatformFeeCents: fee, TeacherAmountCents: teacherAmount,
		Currency: c.Currency, Provider: providerName, Status: repo.PaymentPending,
		PromoCode: promoCode, DiscountCents: discountCents,
		Description: "Оплата курса: " + c.Title, ReturnURL: payments.DefaultReturnURL(),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить попытку оплаты")
		return
	}
	a.checkoutPayment(w, r, p, http.StatusCreated)
}

func (a *App) checkoutPayment(w http.ResponseWriter, r *http.Request, p *repo.Payment, status int) {
	if p.ProviderPaymentID != "" {
		redirect := p.RedirectURL
		if redirect == "" {
			redirect = payments.ResumeURL(p.Provider, p.ProviderPaymentID)
		}
		if redirect == "" {
			writeErr(w, http.StatusConflict, "Ссылка на оплату недоступна. Проверьте статус платежа или обратитесь в поддержку")
			return
		}
		writeJSON(w, status, enrollmentResponse(p, redirect, false))
		return
	}
	prov, err := payments.NewProvider(p.Provider)
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
		PaymentID: p.ID, AmountCents: p.AmountCents, Currency: p.Currency,
		Description: p.Description, ReturnURL: p.ReturnURL,
	})
	if err != nil {
		// Детали апстрима — только в лог, наружу общий текст.
		log.Printf("платёжный провайдер %s: CreatePayment: %v", p.Provider, err)
		writeErr(w, http.StatusBadGateway, "Платёжный сервис недоступен")
		return
	}
	if err := a.Store.SetPaymentCheckout(p.ID, res.ProviderPaymentID, res.RedirectURL); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить результат оплаты, повторите запрос")
		return
	}
	p, found := a.Store.GetPaymentByID(p.ID)
	if !found {
		writeErr(w, http.StatusInternalServerError, "Не удалось прочитать платёж")
		return
	}
	writeJSON(w, status, enrollmentResponse(p, res.RedirectURL, false))
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
	providerName := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "provider")))
	if providerName == "" {
		// Та же семантика, что у payments.NewProvider: пусто = mock.
		providerName = "mock"
	}
	// Defense-in-depth: mock в production отключён полностью — и создание
	// платежа, и приём вебхука (см. CreateEnrollment).
	if providerName == "mock" && a.mockPaymentsDisabled() {
		writeErr(w, http.StatusServiceUnavailable, mockPaymentsDisabledMsg)
		return
	}
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
	// P5: проверка статуса платежа и его применение — одна критическая секция
	// под a.webhookMu. Без замка одновременные доставки одного события обе
	// видели pending и обе отвечали «processed» (гонка, которую ловит тест T35
	// TestPaymentsHardeningConcurrentWebhookRace). Чтение — внутри замка,
	// чтобы не работать со снимком, устаревшим к моменту захвата.
	a.webhookMu.Lock()
	defer a.webhookMu.Unlock()
	unlock, locked := a.lockBilling(w, r)
	if !locked {
		return
	}
	defer unlock()
	p, found := a.Store.GetPaymentByProviderID(event.ProviderPaymentID)
	if !found {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	// Вебхук должен принадлежать платежу: подмена {provider} в URL не должна
	// применять событие чужого провайдера (например, mock-вебхук к stripe-платежу).
	if !strings.EqualFold(strings.TrimSpace(p.Provider), providerName) {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	status, err := a.applyPaymentEvent(p, event.Status)
	if err != nil {
		log.Printf("вебхук %s, платёж %s: %v", providerName, p.ProviderPaymentID, err)
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить статус платежа")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

// mockPaymentsDisabled сообщает, что mock-провайдер запрещён вне dev-режима.
// Страница подтверждения mock-платежа не знает серверный секрет и не может
// подписать вебхук, поэтому разрешать mock в production даже с секретом
// небезопасно: production обязан использовать stripe/yookassa.
func (a *App) mockPaymentsDisabled() bool {
	return !a.Config.IsDev()
}

const mockPaymentsDisabledMsg = "Mock-платёжный провайдер отключён в production: используйте stripe или yookassa"

// applyPaymentEvent — явная state machine статусов платежа. Вызывается под
// a.webhookMu. Возвращает контрактный статус обработки:
//
//	succeeded: pending|failed -> processed, succeeded -> duplicate, refunded -> ignored
//	failed:    pending -> processed, failed -> duplicate, succeeded|refunded -> ignored
//	refunded:  succeeded -> processed, refunded -> duplicate, pending|failed -> ignored
//	pending/неизвестный -> ignored
//
// Побочные эффекты (промокод, доступ) выполняются только при реальном
// переходе в succeeded; duplicate/ignored не меняют платёж в Store и доступ.
// Ошибка persistence возвращается наверх: провайдер повторит вебхук, а
// duplicate-ветки идемпотентно «дочинивают» доступ, если прошлый ответ 500
// пришёл уже после смены статуса платежа.
func (a *App) applyPaymentEvent(p *repo.Payment, eventStatus string) (string, error) {
	if transactional, ok := a.Store.(interface {
		ApplyPaymentOutcome(string, string) (string, error)
	}); ok {
		return transactional.ApplyPaymentOutcome(p.ID, eventStatus)
	}
	switch eventStatus {
	case "succeeded":
		switch p.Status {
		case repo.PaymentSucceeded:
			// A manual revoke must survive retries of an old success event.
			if enrollment, exists := a.Store.GetEnrollment(p.StudentID, p.CourseID); exists && enrollment.Status == repo.EnrollmentRevoked {
				return "duplicate", nil
			}
			if err := a.ensureEnrollmentAccess(p); err != nil {
				return "", err
			}
			return "duplicate", nil
		case repo.PaymentPending, repo.PaymentFailed:
			if !a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded) {
				return "", errors.New("не удалось перевести платёж в succeeded")
			}
			// T63: usedCount увеличивается ровно один раз — при первом
			// реальном переходе в succeeded. Повторные/запоздалые события
			// сюда не доходят (duplicate/ignored выше и ниже).
			if p.PromoCode != "" {
				if promo, ok := a.Store.FindPromoCode(p.CourseID, p.PromoCode); ok {
					if !a.Store.IncrementPromoCodeUsage(promo.ID) {
						log.Printf("вебхук: не удалось увеличить usedCount промокода %s", promo.ID)
					}
				}
			}
			if err := a.ensureEnrollmentAccess(p); err != nil {
				return "", err
			}
			return "processed", nil
		default: // refunded: позднее succeeded не воскрешает доступ
			return "ignored", nil
		}
	case "failed":
		switch p.Status {
		case repo.PaymentFailed:
			return "duplicate", nil
		case repo.PaymentPending:
			if !a.Store.SetPaymentStatus(p.ID, repo.PaymentFailed) {
				return "", errors.New("не удалось перевести платёж в failed")
			}
			return "processed", nil
		default: // succeeded|refunded: не отзываем уже оплаченный доступ
			return "ignored", nil
		}
	case "refunded":
		switch p.Status {
		case repo.PaymentRefunded:
			if err := a.revokeEnrollmentAccess(p); err != nil {
				return "", err
			}
			return "duplicate", nil
		case repo.PaymentSucceeded:
			if !a.Store.SetPaymentStatus(p.ID, repo.PaymentRefunded) {
				return "", errors.New("не удалось перевести платёж в refunded")
			}
			if err := a.revokeEnrollmentAccess(p); err != nil {
				return "", err
			}
			return "processed", nil
		default: // pending|failed: возвращать нечего
			return "ignored", nil
		}
	default:
		// pending и неизвестные статусы не меняют платёж.
		return "ignored", nil
	}
}

// ensureEnrollmentAccess открывает доступ, проверяя, что Store вернул запись.
// nil означает ошибку БД: PostgresStore в этом случае возвращает nil, а не
// фиктивную active-запись.
func (a *App) ensureEnrollmentAccess(p *repo.Payment) error {
	e, _ := a.Store.EnsureEnrollment(p.StudentID, p.CourseID)
	if e == nil {
		return errors.New("не удалось открыть доступ к курсу")
	}
	return nil
}

// revokeEnrollmentAccess отзывает доступ при возврате. Отсутствие записи —
// не ошибка (например, платёж существовал до появления enrollment).
func (a *App) revokeEnrollmentAccess(p *repo.Payment) error {
	// A refund for an earlier purchase cannot revoke another paid purchase
	// or overwrite an administrator's explicit access decision.
	if enrollment, exists := a.Store.GetEnrollment(p.StudentID, p.CourseID); exists && enrollment.Status == repo.EnrollmentRevoked {
		return nil
	}
	for _, other := range a.Store.ListPaymentsByStudent(p.StudentID) {
		if other.ID != p.ID && other.CourseID == p.CourseID && other.Status == repo.PaymentSucceeded {
			return nil
		}
	}
	err := a.Store.SetEnrollmentStatus(p.StudentID, p.CourseID, repo.EnrollmentRefunded)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return fmt.Errorf("не удалось отозвать доступ: %w", err)
	}
	return nil
}

// writeWebhookErr мапит ошибки проверки подписи/провайдера на HTTP-коды:
// 401 — неверная подпись, 502 — внешний платёжный сервис недоступен, 400 — прочее.
func writeWebhookErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrUnauthorized):
		writeErr(w, http.StatusUnauthorized, "Недействительная подпись вебхука")
	case errors.Is(err, payments.ErrUpstream):
		log.Printf("платёжный вебхук: upstream: %v", err)
		writeErr(w, http.StatusBadGateway, "Платёжный сервис недоступен")
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}
