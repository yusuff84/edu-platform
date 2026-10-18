package contract_test

// Контрактные тесты жёсткости платёжного контура (T35) на границе API:
// подделка подписи вебхука, повторная доставка (идемпотентность), гонка
// 8 параллельных вебхуков, неизвестный платёж/провайдер и preflight OPTIONS.
//
// Уровень — полный HTTP-роутер (newTestApp/newTestAppWithCfg), как в
// payments_test.go и auth_hardening_test.go: тесты бьют по реальному коду
// handler/enroll.go (PaymentWebhook) и internal/payments/*.
//
// Подпись mock-провайдера включается переменной окружения MOCK_WEBHOOK_SECRET
// (см. internal/payments/mock.go): без секрета mock её не требует (dev-режим),
// поэтому в тестах подделки секрет задаётся через t.Setenv.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// mockSignature — ожидаемая подпись mock-провайдера: hex(HMAC-SHA256(секрет,
// сырое тело запроса)) — ровно то, что проверяет MockProvider.VerifyWebhook.
func mockSignature(secret string, raw []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

// beginCheckout — платная покупка курса: создаёт платёж в статусе pending и
// возвращает его providerPaymentId (внешний id для вебхуков).
func beginCheckout(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("покупка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	pid, _ := out["providerPaymentId"].(string)
	if pid == "" {
		t.Fatalf("нет providerPaymentId: %s", rec.Body.String())
	}
	return pid
}

// webhookMock шлёт вебхук mock-провайдера без дополнительных заголовков.
func webhookMock(t *testing.T, router http.Handler, providerPaymentID, status string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"providerPaymentId": providerPaymentID, "status": status})
	if err != nil {
		t.Fatalf("marshal webhook: %v", err)
	}
	return doRequestHeaders(t, router, http.MethodPost, "/api/v1/payments/webhook/mock",
		json.RawMessage(raw), nil, nil)
}

// myCoursesTotal — сколько курсов доступно ученику (GET /me/courses).
func myCoursesTotal(t *testing.T, router http.Handler, cookies []*http.Cookie) int {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	total, _ := out["total"].(float64)
	return int(total)
}

// assertSinglePayment: у ученика ровно один платёж по курсу и суммы/сплит
// не удвоены (главная проверка повторной доставки и гонки вебхуков).
func assertSinglePayment(t *testing.T, app *handler.App, studentID, courseID string, wantAmount, wantFee, wantTeacher int) {
	t.Helper()
	payments := app.Store.ListPaymentsByStudent(studentID)
	if len(payments) != 1 {
		t.Fatalf("платежей у ученика %d, ждали ровно один: %+v", len(payments), payments)
	}
	p := payments[0]
	if p.CourseID != courseID {
		t.Fatalf("платёж не по тому курсу: %+v", p)
	}
	if p.AmountCents != wantAmount || p.PlatformFeeCents != wantFee || p.TeacherAmountCents != wantTeacher {
		t.Fatalf("суммы/сплит изменились: amount=%d fee=%d teacher=%d (ждали %d/%d/%d)",
			p.AmountCents, p.PlatformFeeCents, p.TeacherAmountCents, wantAmount, wantFee, wantTeacher)
	}
}

// assertSingleEnrollment: ровно одна запись на курс в ожидаемом статусе.
func assertSingleEnrollment(t *testing.T, app *handler.App, studentID, courseID string, want repo.EnrollmentStatus) {
	t.Helper()
	enrollments := app.Store.ListEnrollmentsByStudent(studentID)
	if len(enrollments) != 1 {
		t.Fatalf("записей у ученика %d, ждали ровно одну: %+v", len(enrollments), enrollments)
	}
	e := enrollments[0]
	if e.CourseID != courseID || e.Status != want {
		t.Fatalf("запись на курс: %+v, ждали курс=%s статус=%s", e, courseID, want)
	}
}

// 1. Подделка подписи (T35, п.1). Вебхук с неверной подписью, вообще без
// заголовка подписи и с подписью от другого тела не должен применяться:
// ответ не 2xx (фактический код реализации — 401), статус платежа остаётся
// pending, записи на курс не появляются.
func TestPaymentsHardeningForgedSignatureRejected(t *testing.T) {
	const secret = "секрет-подписи-T35"
	t.Setenv("MOCK_WEBHOOK_SECRET", secret) // mock читает env при каждом вебхуке

	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "forged@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	succeeded, err := json.Marshal(map[string]any{"providerPaymentId": pid, "status": "succeeded"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	refunded, err := json.Marshal(map[string]any{"providerPaymentId": pid, "status": "refunded"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"подпись не совпадает", map[string]string{"X-Mock-Signature": mockSignature("чужой-секрет", succeeded)}},
		{"заголовка подписи нет", nil},
		{"подпись от другого тела", map[string]string{"X-Mock-Signature": mockSignature(secret, refunded)}},
		{"подпись не hex", map[string]string{"X-Mock-Signature": "не-hex-подпись"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequestHeaders(t, router, http.MethodPost, "/api/v1/payments/webhook/mock",
				json.RawMessage(succeeded), nil, tc.headers)
			// Контракт T35 допускает 400/401/403; фактический код — 401
			// (writeWebhookErr → ErrUnauthorized).
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("подделанный вебхук: status=%d body=%s, ждали 401", rec.Code, rec.Body.String())
			}

			// Статус платежа не изменился...
			p, ok := app.Store.GetPaymentByProviderID(pid)
			if !ok || p.Status != repo.PaymentPending {
				t.Fatalf("статус платежа после подделки: %+v", p)
			}
			// ...доступа к курсу не появилось.
			if e, ok := app.Store.GetEnrollment(studentID, courseID); ok && e.Status == repo.EnrollmentActive {
				t.Fatalf("подделанный вебхук открыл доступ: %+v", e)
			}
			if total := myCoursesTotal(t, router, studentCookies); total != 0 {
				t.Fatalf("курс доступен после подделки: total=%d", total)
			}
		})
	}

	// Контроль: та же подпись, посчитанная по фактическому телу, — применяется
	// (значит, отказы выше вызваны именно проверкой подписи, а не телом).
	rec := doRequestHeaders(t, router, http.MethodPost, "/api/v1/payments/webhook/mock",
		json.RawMessage(succeeded), nil, map[string]string{"X-Mock-Signature": mockSignature(secret, succeeded)})
	if rec.Code != http.StatusOK {
		t.Fatalf("корректная подпись: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "processed" {
		t.Fatalf("корректная подпись: status=%v body=%s", out["status"], rec.Body.String())
	}
	if p, _ := app.Store.GetPaymentByProviderID(pid); p == nil || p.Status != repo.PaymentSucceeded {
		t.Fatalf("платёж не активирован: %+v", p)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ не открылся: total=%d", total)
	}
}

// 2. Повторная доставка (T35, п.2): один и тот же успешный вебхук дважды →
// ровно один платёж, ровно одна запись на курс, суммы и сплит не удвоены,
// второй ответ — идемпотентный 2xx со статусом duplicate.
func TestPaymentsHardeningWebhookDuplicateDelivery(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "duplicate@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	rec := webhookMock(t, router, pid, "succeeded")
	if rec.Code != http.StatusOK {
		t.Fatalf("первый вебхук: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "processed" {
		t.Fatalf("первый вебхук: status=%v body=%s", out["status"], rec.Body.String())
	}

	rec = webhookMock(t, router, pid, "succeeded")
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("повторный вебхук: status=%d body=%s, ждали идемпотентный 2xx", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["status"] != "duplicate" {
		t.Fatalf("повторный вебхук: status=%v body=%s", out["status"], rec.Body.String())
	}

	// Ровно один платёж со «своими» суммами и ровно одна активная запись.
	assertSinglePayment(t, app, studentID, courseID, 10000, 1500, 8500)
	assertSingleEnrollment(t, app, studentID, courseID, repo.EnrollmentActive)
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ: total=%d, ждали 1", total)
	}
}

// 3. Гонка (T35, п.3): 8 одновременных доставок одного успешного вебхука →
// ровно один платёж и ровно одна запись на курс, ровно один ответ processed,
// остальные — duplicate. До правки T35 (мьютекс применения события в
// handler/enroll.go) несколько горутин видели статус pending и все отвечали
// processed, а -race фиксировал гонку чтения Payment.Status с записью в store.
func TestPaymentsHardeningConcurrentWebhookRace(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "race@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	const parallel = 8
	body, err := json.Marshal(map[string]any{"providerPaymentId": pid, "status": "succeeded"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	codes := make([]int, parallel)
	labels := make([]string, parallel)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Свой request/recorder на каждую горутину: общий httptest-объект
			// сам по себе был бы гонкой.
			req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			<-start // одновременный старт всех доставок
			router.ServeHTTP(rec, req)
			codes[i] = rec.Code
			var out map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			labels[i], _ = out["status"].(string)
		}(i)
	}
	close(start)
	wg.Wait()

	processed, duplicate := 0, 0
	for i := range codes {
		if codes[i] != http.StatusOK {
			t.Fatalf("доставка %d: status=%d", i, codes[i])
		}
		switch labels[i] {
		case "processed":
			processed++
		case "duplicate":
			duplicate++
		default:
			t.Fatalf("доставка %d: статус %q", i, labels[i])
		}
	}
	if processed != 1 {
		t.Fatalf("processed=%d, duplicate=%d: событие должно примениться ровно один раз", processed, duplicate)
	}
	if duplicate != parallel-1 {
		t.Fatalf("duplicate=%d, ждали %d", duplicate, parallel-1)
	}

	// Платежа и записи ровно по одному, суммы/сплит не удвоились.
	assertSinglePayment(t, app, studentID, courseID, 10000, 1500, 8500)
	assertSingleEnrollment(t, app, studentID, courseID, repo.EnrollmentActive)
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ: total=%d, ждали 1", total)
	}
}

// 4. Неизвестный платёж и провайдер (T35, п.4): вебхук со ссылкой на
// несуществующий платёж → 404; провайдер, которого нет в конфиге → 400.
func TestPaymentsHardeningUnknownPaymentAndProvider(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, _ := registerUser(t, router, "unknown@example.com", "password123", "")
	pid := beginCheckout(t, router, studentCookies, courseID)

	// Несуществующий providerPaymentId → 404.
	rec := webhookMock(t, router, "mock_этого-платежа-нет", "succeeded")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("неизвестный платёж: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
	}

	// Неизвестный провайдер в URL → 400 с русским текстом.
	raw, err := json.Marshal(map[string]any{"providerPaymentId": pid, "status": "succeeded"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec = doRequestHeaders(t, router, http.MethodPost, "/api/v1/payments/webhook/нет-такого-провайдера",
		json.RawMessage(raw), nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("неизвестный провайдер: status=%d body=%s, ждали 400", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if msg, _ := errBody["error"].(string); !strings.Contains(msg, "неизвестный платёжный провайдер") {
		t.Fatalf("текст ошибки неизвестного провайдера: %v", errBody["error"])
	}

	// stripe/yookassa объявлены в конфиге приложения, но без ключей в env это
	// сбой конфигурации, а не запроса: 502. Если ключи всё-таки заданы в
	// окружении, вебхук доходит до проверки подписи и отвечает 401 —
	// фиксируем оба фактически допустимых кода.
	for _, name := range []string{"stripe", "yookassa"} {
		rec = doRequestHeaders(t, router, http.MethodPost, "/api/v1/payments/webhook/"+name,
			json.RawMessage(raw), nil, nil)
		if rec.Code != http.StatusBadGateway && rec.Code != http.StatusUnauthorized {
			t.Fatalf("провайдер %s: status=%d body=%s, ждали 502 (нет ключей) или 401", name, rec.Code, rec.Body.String())
		}
		t.Logf("провайдер %s без ключей: status=%d body=%s", name, rec.Code, rec.Body.String())
	}

	// Живой платёж не тронут ни одним из этих вебхуков.
	if p, ok := app.Store.GetPaymentByProviderID(pid); !ok || p.Status != repo.PaymentPending {
		t.Fatalf("статус платежа изменился: %+v", p)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ появился без оплаты: total=%d", total)
	}
}

// 5. Preflight (T35, п.5): OPTIONS /api/v1/auth/me с разрешённого origin →
// 204 (факт реализации; контракт допускал 200) и полный набор CORS-заголовков;
// OPTIONS с неразрешённого origin → без Access-Control-Allow-Origin.
func TestPaymentsHardeningPreflightCORS(t *testing.T) {
	const allowed = "http://localhost:3000"
	cfg := &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		AppEnv:           "production",
		SessionSecret:    "контрактный-секрет-T35",
		AllowedOrigins:   []string{allowed},
	}
	_, router := newTestAppWithCfg(cfg)

	rec := doRequestHeaders(t, router, http.MethodOptions, "/api/v1/auth/me", nil, nil, map[string]string{
		"Origin":                         allowed,
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "content-type",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight с разрешённого origin: status=%d, ждали 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Fatalf("Access-Control-Allow-Origin=%q, ждали %q", got, allowed)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Fatalf("Access-Control-Allow-Methods=%q, ждали POST в списке", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Content-Type") {
		t.Fatalf("Access-Control-Allow-Headers=%q, ждали Content-Type в списке", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("нет Access-Control-Allow-Credentials=true (нужен для cookie-сессий)")
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Fatalf("Vary=%q, ждали Origin (кэши не должны путать origin)", rec.Header().Get("Vary"))
	}

	// Неразрешённый origin: CORS-заголовков доступа быть не должно.
	rec = doRequestHeaders(t, router, http.MethodOptions, "/api/v1/auth/me", nil, nil, map[string]string{
		"Origin":                        "https://evil.example",
		"Access-Control-Request-Method": "POST",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight с чужого origin: status=%d, ждали 204 без CORS-заголовков", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("чужой origin получил Access-Control-Allow-Origin=%q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("чужой origin получил Access-Control-Allow-Methods=%q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("чужой origin получил Access-Control-Allow-Credentials=%q", got)
	}
}

// 6. Повторная покупка бесплатного курса после возврата средств (баг, найденный
// при написании тестов T35). providerPaymentId бесплатного платежа
// детерминирован ("free-<ученик>-<курс>"), поэтому вторая покупка после
// возврата получала ErrAlreadyExists, p == nil и панику на p.ID → 500.
func TestPaymentsHardeningFreeCourseRepurchaseAfterRefund(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный курс
	studentCookies, studentID := registerUser(t, router, "free@example.com", "password123", "")

	// Первая покупка: 201, платёж сразу succeeded, доступ активен.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("бесплатная покупка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var first map[string]any
	decodeBody(t, rec, &first)
	pid, _ := first["providerPaymentId"].(string)
	if pid == "" || first["status"] != "succeeded" || first["enrollmentActive"] != true {
		t.Fatalf("ответ на бесплатную покупку: %s", rec.Body.String())
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ после бесплатной покупки: total=%d", total)
	}

	// Возврат средств по этому платежу: доступ отзывается.
	rec = webhookMock(t, router, pid, "refunded")
	if rec.Code != http.StatusOK {
		t.Fatalf("вебхук возврата: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("после возврата доступ остался: total=%d", total)
	}

	// Повторная покупка того же бесплатного курса: 201 (раньше — паника/500),
	// платёж переиспользуется, доступ снова активен, дублей платежей нет.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("повторная бесплатная покупка: status=%d body=%s, ждали 201", rec.Code, rec.Body.String())
	}
	var second map[string]any
	decodeBody(t, rec, &second)
	if second["paymentId"] != first["paymentId"] || second["providerPaymentId"] != pid {
		t.Fatalf("повторная покупка создала новый платёж: %s", rec.Body.String())
	}
	if second["enrollmentActive"] != true {
		t.Fatalf("доступ не открылся: %s", rec.Body.String())
	}
	assertSinglePayment(t, app, studentID, courseID, 0, 0, 0)
	assertSingleEnrollment(t, app, studentID, courseID, repo.EnrollmentActive)
	if total := myCoursesTotal(t, router, studentCookies); total != 1 {
		t.Fatalf("доступ после повторной покупки: total=%d", total)
	}
}
