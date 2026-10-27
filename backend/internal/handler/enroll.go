package handler

import (
	"errors"
	"log"
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
	// Provider оставлен для совместимости с фронтендом, но НЕ выбирает
	// провайдера: источник истины — Config.PaymentsProvider (P11). Иначе
	// клиент мог бы попросить mock при PAYMENTS_PROVIDER=stripe и получить
	// тестовый платёж в обход реальной оплаты.
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
			// ниже он возвращается в succeeded, доступ открывает
			// activateFreeEnrollment (явная повторная покупка).
			p = freePaymentByCourse(a.Store, u.ID, c.ID)
			if p == nil {
				writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
				return
			}
		default:
			writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
			return
		}
		// Повторная покупка бесплатного курса после возврата — явное новое
		// действие ученика. Старый детерминированный платёж переиспользуется,
		// поэтому статус и доступ выставляются здесь напрямую; вебхук так
		// делать не может (canTransition запрещает refunded/failed → succeeded).
		if p.Status != repo.PaymentSucceeded {
			if !a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded) {
				writeErr(w, http.StatusInternalServerError, "Не удалось оформить бесплатный курс")
				return
			}
		}
		if !a.activateFreeEnrollment(u.ID, c.ID) {
			writeErr(w, http.StatusInternalServerError, "Не удалось открыть доступ к курсу")
			return
		}
		writeJSON(w, http.StatusCreated, enrollmentResponse(p, "", true))
		return
	}

	// P11: провайдер всегда из конфигурации, клиент не выбирает платёжный
	// канал. Непустой req.Provider оставлен для совместимости с фронтендом,
	// но при расхождении с конфигом это ошибка запроса (400), а не повод
	// молча переключить провайдера. Пустое значение — «как настроено».
	providerName := strings.ToLower(strings.TrimSpace(a.Config.PaymentsProvider))
	if providerName == "" && a.Config.IsDev() {
		// Пустой конфиг в dev означает mock (см. config.Load). Вне dev
		// оставляем пусто: NewProvider вернёт ErrNotConfigured (502), а не
		// молча подменит канал на тестовый.
		providerName = "mock"
	}
	if requested := strings.ToLower(strings.TrimSpace(req.Provider)); requested != "" && requested != providerName {
		writeErr(w, http.StatusBadRequest, "Платёжный провайдер не соответствует настройкам платформы")
		return
	}

	prov, err := payments.NewProvider(providerName, a.Config.IsDev())
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
		AmountCents: amountCents,
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
		AmountCents: amountCents, PlatformFeeCents: fee, TeacherAmountCents: teacherAmount,
		Currency: c.Currency, Provider: prov.Name(),
		ProviderPaymentID: res.ProviderPaymentID,
		Status:            repo.PaymentPending,
		PromoCode:         promoCode, DiscountCents: discountCents,
	})
	if err != nil {
		// P11: 409 — только реальный дубль providerPaymentId. Прочие сбои
		// записи (БД недоступна и т.п.) — 500, а не «платёж уже создан».
		// Внешний платёж к этому моменту уже создан, но доступ не открываем
		// и ошибку не теряем.
		if errors.Is(err, repo.ErrAlreadyExists) {
			writeErr(w, http.StatusConflict, "Такой платёж уже создан")
			return
		}
		// M6: внешний платёж уже создан, а записи в БД нет. Автоматический
		// Refund здесь небезопасен (у Stripe/YooKassa платёж может быть ещё
		// не оплачен, а ошибка БД — неоднозначной после commit). Фиксируем
		// providerPaymentId в логе для ручной сверки/компенсации.
		log.Printf("P11: платёж провайдера %s (%s) создан, но не сохранён в БД: %v; нужна сверка",
			res.ProviderPaymentID, prov.Name(), err)
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить платёж")
		return
	}
	writeJSON(w, http.StatusCreated, enrollmentResponse(p, res.RedirectURL, false))
}

// canTransition — разрешённые переходы статуса платежа (P11):
// pending → succeeded|failed; succeeded → refunded. refunded и failed
// терминальны: повторный succeeded не должен ни реактивировать доступ, ни
// повторно увеличивать usedCount промокода.
func canTransition(from, to repo.PaymentStatus) bool {
	switch from {
	case repo.PaymentPending:
		return to == repo.PaymentSucceeded || to == repo.PaymentFailed
	case repo.PaymentSucceeded:
		return to == repo.PaymentRefunded
	default:
		return false
	}
}

// activateEnrollment открывает доступ после успешной оплаты.
//
// allowReactivate=true — только для перехода pending → succeeded (новая
// оплата, в том числе повторная покупка после возврата или отзыва доступа):
// refunded/revoked-запись можно вернуть в active, потому что ученик явно
// заплатил снова. allowReactivate=false — для duplicate succeeded: повторная
// доставка события не должна реактивировать возвращённую или отозванную
// запись.
//
// Отсутствующая запись создаётся через EnsureEnrollment; результат проверяется
// повторным чтением, потому что Postgres-реализация при сбое записи может
// вернуть фиктивную структуру.
func (a *App) activateEnrollment(studentID, courseID string, allowReactivate bool) bool {
	if e, ok := a.Store.GetEnrollment(studentID, courseID); ok {
		switch e.Status {
		case repo.EnrollmentActive:
			return true
		case repo.EnrollmentRefunded, repo.EnrollmentRevoked:
			if !allowReactivate {
				return true // повторный succeeded не возвращает доступ
			}
			return a.Store.SetEnrollmentStatus(studentID, courseID, repo.EnrollmentActive) == nil
		default:
			return true
		}
	}
	if e, _ := a.Store.EnsureEnrollment(studentID, courseID); e == nil {
		return false
	}
	got, ok := a.Store.GetEnrollment(studentID, courseID)
	return ok && got.Status == repo.EnrollmentActive
}

// activateFreeEnrollment — открытие доступа при повторной покупке бесплатного
// курса. В отличие от вебхука это явное новое действие ученика, поэтому
// refunded/revoked-запись можно вернуть в active; ошибка записи — false
// (handler 500).
func (a *App) activateFreeEnrollment(studentID, courseID string) bool {
	if e, ok := a.Store.GetEnrollment(studentID, courseID); ok {
		switch e.Status {
		case repo.EnrollmentActive:
			return true
		default:
			return a.Store.SetEnrollmentStatus(studentID, courseID, repo.EnrollmentActive) == nil
		}
	}
	if e, _ := a.Store.EnsureEnrollment(studentID, courseID); e == nil {
		return false
	}
	got, ok := a.Store.GetEnrollment(studentID, courseID)
	return ok && got.Status == repo.EnrollmentActive
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
	providerName := strings.TrimSpace(chi.URLParam(r, "provider"))
	// P11: mock-вебхук — только dev. Вне dev отвечаем 404 так же, как на
	// неизвестный платёж: маршрут существует, но тестовый провайдер выключен.
	if strings.EqualFold(providerName, "mock") && !a.Config.IsDev() {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	prov, err := payments.NewProvider(providerName, a.Config.IsDev())
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
	p, found := a.Store.GetPaymentByProviderID(event.ProviderPaymentID)
	if !found {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	// P11 (cross-provider): платёж, созданный Stripe/YooKassa, нельзя
	// подтвердить через mock-вебхук (и наоборот). Иначе клиент мог бы открыть
	// доступ, послав событие «succeeded» чужому провайдеру.
	if !strings.EqualFold(p.Provider, prov.Name()) {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	switch event.Status {
	case "succeeded":
		if p.Status == repo.PaymentSucceeded {
			// Идемпотентный repair: первый webhook мог применить succeeded,
			// но не успеть создать запись на курс. EnsureEnrollment создаст
			// отсутствующую запись; refunded/revoked не реактивируются.
			if !a.activateEnrollment(p.StudentID, p.CourseID, false) {
				writeErr(w, http.StatusInternalServerError, "Не удалось открыть доступ к курсу")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
			return
		}
		if !canTransition(p.Status, repo.PaymentSucceeded) {
			// refunded/failed терминальны: повторный succeeded не возвращает
			// доступ и не увеличивает usedCount.
			writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
			return
		}
		if !a.Store.SetPaymentStatus(p.ID, repo.PaymentSucceeded) {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить статус платежа")
			return
		}
		// T63: usedCount промокода увеличивается ровно один раз — здесь, при
		// первом переходе платежа в succeeded. Ветка duplicate выше уже
		// отсекла повторную доставку события (та же идемпотентность по
		// providerPaymentID), поэтому счётчик не задваивается. Возврат
		// средств его не уменьшает (ветка refunded ниже).
		if p.PromoCode != "" {
			if promo, ok := a.Store.FindPromoCode(p.CourseID, p.PromoCode); ok {
				if !a.Store.IncrementPromoCodeUsage(promo.ID) {
					// false — либо код исчез (сбой данных), либо лимит уже
					// исчерпан (гонка с другим платежом/поздний webhook).
					// Лимит не превышаем: платёж уже succeeded, доступ
					// открываем, но usedCount не растёт. Остальные причины —
					// ошибка записи, её нельзя выдавать за 200 processed.
					current, exists := a.Store.GetPromoCodeByID(promo.ID)
					if !exists || current.MaxUses <= 0 || current.UsedCount < current.MaxUses {
						writeErr(w, http.StatusInternalServerError, "Не удалось учесть промокод")
						return
					}
				}
			}
		}
		// pending → succeeded: новая оплата. Повторная покупка после возврата
		// или отзыва доступа законно возвращает запись в active.
		if !a.activateEnrollment(p.StudentID, p.CourseID, true) {
			writeErr(w, http.StatusInternalServerError, "Не удалось открыть доступ к курсу")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	case "failed":
		if p.Status == repo.PaymentFailed {
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
			return
		}
		if !canTransition(p.Status, repo.PaymentFailed) {
			// succeeded → failed и refunded → failed не применяем: событие
			// устарело или пришло не по порядку, доступ отбирать нельзя.
			writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
			return
		}
		if !a.Store.SetPaymentStatus(p.ID, repo.PaymentFailed) {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить статус платежа")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "processed"})
	case "refunded":
		if p.Status == repo.PaymentRefunded {
			// Repair: если первый webhook успел перевести платёж в refunded,
			// но не смог закрыть доступ (500 → retry), повторная доставка
			// должна довести отзыв доступа, а не просто вернуть duplicate.
			if err := a.Store.SetEnrollmentStatus(p.StudentID, p.CourseID, repo.EnrollmentRefunded); err != nil && !errors.Is(err, repo.ErrNotFound) {
				writeErr(w, http.StatusInternalServerError, "Не удалось закрыть доступ к курсу")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate"})
			return
		}
		if !canTransition(p.Status, repo.PaymentRefunded) {
			// Возвращать можно только оплаченный платёж.
			writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
			return
		}
		if !a.Store.SetPaymentStatus(p.ID, repo.PaymentRefunded) {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить статус платежа")
			return
		}
		// Возврат средств отменяет доступ: запись на курс становится refunded.
		// ErrNotFound не считается сбоем (записи нет — отзывать нечего),
		// остальные ошибки — 500, чтобы провайдер повторил webhook.
		if err := a.Store.SetEnrollmentStatus(p.StudentID, p.CourseID, repo.EnrollmentRefunded); err != nil && !errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusInternalServerError, "Не удалось закрыть доступ к курсу")
			return
		}
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
