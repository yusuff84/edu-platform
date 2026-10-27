package contract_test

// P11: критичные дефекты платёжной безопасности на границе API.
//   - чужой провайдер не может подтвердить платёж своим вебхуком;
//   - клиент не выбирает платёжного провайдера (всегда Config.PaymentsProvider);
//   - mock запрещён вне dev (страница и вебхук недоступны);
//   - refunded/failed терминальны: повторный succeeded не открывает доступ;
//   - duplicate succeeded чинит отсутствующую запись на курс;
//   - succeeded→failed не оставляет несогласованного состояния.

import (
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// 1. Платёж Stripe/YooKassa нельзя подтвердить через mock-вебхук: 404 и
// статус остаётся pending.
func TestP11CrossProviderWebhookRejected(t *testing.T) {
	cases := []struct {
		provider string
		payment  string
	}{
		{"stripe", "pi_p11_cross_provider"},
		{"yookassa", "yk_p11_cross_provider"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			app, router := newTestApp()
			_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
			studentCookies, studentID := registerUser(t, router,
				"p11-cross-"+tc.provider+"@example.com", "password123", "")

			p, err := app.Store.CreatePayment(&repo.Payment{
				StudentID: studentID, CourseID: courseID,
				AmountCents: 10000, Currency: "RUB",
				Provider: tc.provider, ProviderPaymentID: tc.payment,
				Status: repo.PaymentPending,
			})
			if err != nil {
				t.Fatalf("CreatePayment(%s): %v", tc.provider, err)
			}

			rec := webhookMock(t, router, p.ProviderPaymentID, "succeeded")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("cross-provider webhook: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
			}
			got, ok := app.Store.GetPaymentByProviderID(p.ProviderPaymentID)
			if !ok || got.Status != repo.PaymentPending {
				t.Fatalf("статус чужого платежа изменился: %+v", got)
			}
			if e, ok := app.Store.GetEnrollment(studentID, courseID); ok && e.Status == repo.EnrollmentActive {
				t.Fatalf("cross-provider webhook открыл доступ: %+v", e)
			}
			if total := myCoursesTotal(t, router, studentCookies); total != 0 {
				t.Fatalf("курс доступен без оплаты: total=%d", total)
			}
		})
	}
}

// 2. Клиентский provider:"mock" при PAYMENTS_PROVIDER=stripe — 400
// (расхождение с конфигом), платёж не создаётся и провайдер не подменяется.
func TestP11ClientCannotChooseMockProvider(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")

	cfg := &config.Config{
		PaymentsProvider: "stripe",
		CommissionPct:    15,
		AppEnv:           "development",
		AllowedOrigins:   []string{},
	}
	app, router := newTestAppWithCfg(cfg)
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "p11-provider@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("override provider: status=%d body=%s, ждали 400 (расхождение с конфигом)", rec.Code, rec.Body.String())
	}
	if payments := app.Store.ListPaymentsByStudent(studentID); len(payments) != 0 {
		t.Fatalf("клиентский provider создал платёж: %+v", payments)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); ok && e.Status == repo.EnrollmentActive {
		t.Fatalf("доступ открылся без оплаты: %+v", e)
	}
}

// 3. Вне dev mock полностью выключен: страница и вебхук — 404, покупка через
// mock-провайдера — 502, платёж не создаётся.
func TestP11MockDisabledOutsideDev(t *testing.T) {
	cfg := &config.Config{
		PaymentsProvider:       "mock",
		CommissionPct:          15,
		AppEnv:                 "production",
		DatabaseURL:            "postgres://edu:edu@db:5432/edu",
		SessionSecret:          "п11-прод-секрет",
		AllowedOrigins:         []string{},
		CSRFAllowMissingOrigin: true,
	}
	app, router := newTestAppWithCfg(cfg)
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "p11-prod@example.com", "password123", "")

	if rec := doRequest(t, router, http.MethodGet, "/pay/mock/mock_p11", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("mock-страница в prod: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
	}
	rec := webhookMock(t, router, "mock_p11", "succeeded")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mock-вебхук в prod: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("mock-покупка в prod: status=%d body=%s, ждали 502", rec.Code, rec.Body.String())
	}
	if payments := app.Store.ListPaymentsByStudent(studentID); len(payments) != 0 {
		t.Fatalf("в prod создан mock-платёж: %+v", payments)
	}
}

// 4. refunded → succeeded не реактивирует доступ: платёж остаётся refunded,
// запись — refunded, повторного processed нет.
func TestP11RefundedSucceededDoesNotReactivate(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "p11-refunded@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	if rec := webhookMock(t, router, pid, "succeeded"); rec.Code != http.StatusOK {
		t.Fatalf("succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := webhookMock(t, router, pid, "refunded"); rec.Code != http.StatusOK {
		t.Fatalf("refunded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); !ok || e.Status != repo.EnrollmentRefunded {
		t.Fatalf("после возврата запись: %+v", e)
	}

	rec := webhookMock(t, router, pid, "succeeded")
	if rec.Code != http.StatusOK {
		t.Fatalf("повтор succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "ignored" {
		t.Fatalf("повтор succeeded после refunded: status=%v, ждали ignored", out["status"])
	}
	if p, _ := app.Store.GetPaymentByProviderID(pid); p == nil || p.Status != repo.PaymentRefunded {
		t.Fatalf("платёж реактивирован: %+v", p)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); !ok || e.Status != repo.EnrollmentRefunded {
		t.Fatalf("запись реактивирована: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ вернулся после возврата: total=%d", total)
	}
}

// 5. failed → succeeded не открывает доступ: failed терминален.
func TestP11FailedSucceededDoesNotReactivate(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "p11-failed@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	if rec := webhookMock(t, router, pid, "failed"); rec.Code != http.StatusOK {
		t.Fatalf("failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec := webhookMock(t, router, pid, "succeeded")
	if rec.Code != http.StatusOK {
		t.Fatalf("succeeded после failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "ignored" {
		t.Fatalf("succeeded после failed: status=%v, ждали ignored", out["status"])
	}
	if p, _ := app.Store.GetPaymentByProviderID(pid); p == nil || p.Status != repo.PaymentFailed {
		t.Fatalf("платёж после failed: %+v", p)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); ok && e.Status == repo.EnrollmentActive {
		t.Fatalf("failed→succeeded открыл доступ: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ появился после failed: total=%d", total)
	}
}

// 6. succeeded → failed не применяется (устаревшее событие): платёж остаётся
// succeeded, доступ — active, то есть нет несогласованного «failed + active».
func TestP11SucceededFailedDoesNotLeaveInconsistentState(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "p11-succ-fail@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	if rec := webhookMock(t, router, pid, "succeeded"); rec.Code != http.StatusOK {
		t.Fatalf("succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec := webhookMock(t, router, pid, "failed")
	if rec.Code != http.StatusOK {
		t.Fatalf("failed после succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "ignored" {
		t.Fatalf("failed после succeeded: status=%v, ждали ignored", out["status"])
	}
	if p, _ := app.Store.GetPaymentByProviderID(pid); p == nil || p.Status != repo.PaymentSucceeded {
		t.Fatalf("платёж стал failed: %+v", p)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); !ok || e.Status != repo.EnrollmentActive {
		t.Fatalf("доступ сломан устаревшим failed: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ пропал из-за устаревшего failed: total=%d", total)
	}
}

// 7. duplicate succeeded чинит отсутствующую запись на курс (идемпотентный
// repair), а не просто отвечает duplicate.
func TestP11DuplicateSucceededRepairsEnrollment(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	_, studentID := registerUser(t, router, "p11-repair@example.com", "password123", "")

	p, err := app.Store.CreatePayment(&repo.Payment{
		StudentID: studentID, CourseID: courseID,
		AmountCents: 10000, Currency: "RUB",
		Provider: "mock", ProviderPaymentID: "mock_p11_repair",
		Status: repo.PaymentSucceeded,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if _, ok := app.Store.GetEnrollment(studentID, courseID); ok {
		t.Fatal("предусловие: записи на курс быть не должно")
	}

	for i := 0; i < 2; i++ {
		rec := webhookMock(t, router, p.ProviderPaymentID, "succeeded")
		if rec.Code != http.StatusOK {
			t.Fatalf("duplicate succeeded #%d: status=%d body=%s", i+1, rec.Code, rec.Body.String())
		}
		var out map[string]any
		decodeBody(t, rec, &out)
		if out["status"] != "duplicate" {
			t.Fatalf("duplicate succeeded #%d: status=%v", i+1, out["status"])
		}
	}
	e, ok := app.Store.GetEnrollment(studentID, courseID)
	if !ok || e.Status != repo.EnrollmentActive {
		t.Fatalf("repair не создал активную запись: %+v", e)
	}
	if got := app.Store.ListEnrollmentsByStudent(studentID); len(got) != 1 {
		t.Fatalf("записей после repair %d, ждали одну: %+v", len(got), got)
	}
}

// 8. failed-вебхук сам по себе не активирует запись на курс и не открывает
// доступ, даже если это первый и единственный статус платежа.
func TestP11FailedPaymentDoesNotActivateEnrollment(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "p11-failed-only@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	rec := webhookMock(t, router, pid, "failed")
	if rec.Code != http.StatusOK {
		t.Fatalf("failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "processed" {
		t.Fatalf("failed: status=%v, ждали processed", out["status"])
	}
	if p, _ := app.Store.GetPaymentByProviderID(pid); p == nil || p.Status != repo.PaymentFailed {
		t.Fatalf("платёж не стал failed: %+v", p)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); ok && e.Status == repo.EnrollmentActive {
		t.Fatalf("failed-платёж активировал запись: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("failed-платёж открыл доступ: total=%d", total)
	}
}

// 9. Mock-страница не открывает платёж чужого провайдера: вместо суммы и
// названия курса — «Платёж не найден», как для неизвестного id.
func TestP11MockPageDoesNotOpenForeignPayment(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	_, studentID := registerUser(t, router, "p11-page@example.com", "password123", "")

	if _, err := app.Store.CreatePayment(&repo.Payment{
		StudentID: studentID, CourseID: courseID,
		AmountCents: 10000, Currency: "RUB",
		Provider: "stripe", ProviderPaymentID: "pi_p11_page_foreign",
		Status: repo.PaymentPending,
	}); err != nil {
		t.Fatalf("CreatePayment(stripe): %v", err)
	}

	rec := doRequest(t, router, http.MethodGet, "/pay/mock/pi_p11_page_foreign", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("mock-страница чужого платежа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Платёж не найден") {
		t.Fatalf("чужой платёж не скрыт:\n%s", body)
	}
	if strings.Contains(body, "Go для новичков") || strings.Contains(body, "100.00") {
		t.Fatalf("mock-страница раскрыла данные чужого платежа:\n%s", body)
	}
}

// 10. maxUses промокода не превышается, даже если succeeded-вебхуки пришли по
// двум уже созданным платежам с одним кодом (лимит = 1).
func TestP11PromoMaxUsesNotExceeded(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	_, studentID := registerUser(t, router, "p11-promo@example.com", "password123", "")

	promo, err := app.Store.CreatePromoCode(&repo.PromoCode{
		CourseID: courseID, Code: "LIMIT1", DiscountPct: 10, MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode: %v", err)
	}
	for i, providerID := range []string{"mock_p11_promo_1", "mock_p11_promo_2"} {
		if _, err := app.Store.CreatePayment(&repo.Payment{
			StudentID: studentID, CourseID: courseID,
			AmountCents: 9000, Currency: "RUB", Provider: "mock",
			ProviderPaymentID: providerID, Status: repo.PaymentPending,
			PromoCode: promo.Code, DiscountCents: 1000,
		}); err != nil {
			t.Fatalf("CreatePayment #%d: %v", i+1, err)
		}
		rec := webhookMock(t, router, providerID, "succeeded")
		if rec.Code != http.StatusOK {
			t.Fatalf("webhook #%d: status=%d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}

	got, ok := app.Store.GetPromoCodeByID(promo.ID)
	if !ok || got.UsedCount != 1 {
		t.Fatalf("usedCount=%+v, ждали 1 (maxUses не превышен)", got)
	}
}
