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
// остальные — duplicate. Гонка «несколько горутин видят pending и все
// отвечают processed» закрыта в P5: применение события в handler/enroll.go
// выполняется под a.webhookMu (проверка статуса и запись — одна секция).
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

// 5. Preflight (T35, п.5; путь уточнён T42): OPTIONS реального POST-маршрута
// (/api/v1/auth/login) с разрешённого origin → 204 (факт реализации; контракт
// допускал 200) и полный набор CORS-заголовков; с T42 Allow-Methods
// перечисляет методы именно этого пути: POST есть, GET (не зарегистрирован
// у /auth/login) — нет. OPTIONS с неразрешённого origin → без
// Access-Control-Allow-Origin.
func TestPaymentsHardeningPreflightCORS(t *testing.T) {
	const allowed = "http://localhost:3000"
	const preflightPath = "/api/v1/auth/login" // реальный POST-маршрут (T42)
	cfg := &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		AppEnv:           "production",
		SessionSecret:    "контрактный-секрет-T35",
		AllowedOrigins:   []string{allowed},
	}
	_, router := newTestAppWithCfg(cfg)

	rec := doRequestHeaders(t, router, http.MethodOptions, preflightPath, nil, nil, map[string]string{
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
	if got := rec.Header().Get("Access-Control-Allow-Methods"); strings.Contains(got, "GET") {
		t.Fatalf("Access-Control-Allow-Methods=%q: у POST-маршрута /auth/login не должно быть GET (T42)", got)
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
	rec = doRequestHeaders(t, router, http.MethodOptions, preflightPath, nil, nil, map[string]string{
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

// productionPaymentsCfg — конфиг production-режима для проверок политики
// mock-провайдера. CSRFAllowMissingOrigin нужен тестовым POST без браузера.
func productionPaymentsCfg(provider string) *config.Config {
	return &config.Config{
		PaymentsProvider:       provider,
		CommissionPct:          15,
		AppEnv:                 "production",
		SessionSecret:          "contract-secret-payments-hardening",
		AllowedOrigins:         []string{},
		CSRFAllowMissingOrigin: true,
	}
}

// 7. Провайдера выбирает только сервер (критический security-фикс): поле
// provider в запросе оставлено для совместимости, но не влияет на выбор.
func TestPaymentsHardeningClientCannotChooseProvider(t *testing.T) {
	t.Setenv("MOCK_WEBHOOK_SECRET", "")

	t.Run("конфиг mock: клиентский stripe игнорируется", func(t *testing.T) {
		app, router := newTestApp()
		_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
		studentCookies, studentID := registerUser(t, router, "provider-mock@example.com", "password123", "")

		rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
			"courseId": courseID, "provider": "stripe",
		}, studentCookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("enroll с клиентским stripe: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		decodeBody(t, rec, &out)
		pid, _ := out["providerPaymentId"].(string)
		if !strings.HasPrefix(pid, "mock_") {
			t.Fatalf("клиентский stripe создал не mock-платёж: %s", rec.Body.String())
		}
		p, ok := app.Store.GetPaymentByProviderID(pid)
		if !ok || p.Provider != "mock" {
			t.Fatalf("провайдер должен быть серверным mock: %+v", p)
		}
		if got := app.Store.ListPaymentsByStudent(studentID); len(got) != 1 {
			t.Fatalf("платежей %d, ждали ровно один", len(got))
		}
	})

	t.Run("конфиг stripe: клиентский mock не создаёт mock-платёж", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "")
		t.Setenv("STRIPE_WEBHOOK_SECRET", "")
		cfg := &config.Config{
			PaymentsProvider: "stripe",
			CommissionPct:    15,
			AllowedOrigins:   []string{},
		}
		app, router := newTestAppWithCfg(cfg)
		_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
		studentCookies, studentID := registerUser(t, router, "provider-stripe@example.com", "password123", "")

		rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
			"courseId": courseID, "provider": "mock",
		}, studentCookies)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("клиентский mock при конфиге stripe: status=%d body=%s, ждали 502 (stripe без ключей), не 201",
				rec.Code, rec.Body.String())
		}
		var errBody map[string]any
		decodeBody(t, rec, &errBody)
		if msg, _ := errBody["error"].(string); !strings.Contains(msg, "не настроен") {
			t.Fatalf("текст ошибки конфигурации: %v", errBody["error"])
		}
		if got := app.Store.ListPaymentsByStudent(studentID); len(got) != 0 {
			t.Fatalf("платёж создан при ненастроенном stripe: %+v", got)
		}
	})
}

// 8. Mock в production запрещён полностью и независимо от секрета: платёж не
// создаётся, а даже заранее созданный mock-платёж нельзя активировать
// публичным вебхуком. Выбранный контракт — 503 и понятный русский текст.
func TestPaymentsHardeningMockForbiddenInProduction(t *testing.T) {
	t.Setenv("MOCK_WEBHOOK_SECRET", "")

	app, router := newTestAppWithCfg(productionPaymentsCfg("mock"))
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "prod-mock-denied@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("mock в production: status=%d body=%s, ждали 503", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if msg, _ := errBody["error"].(string); !strings.Contains(msg, "stripe или yookassa") {
		t.Fatalf("текст ошибки должен объяснять причину: %v", errBody["error"])
	}
	if got := app.Store.ListPaymentsByStudent(studentID); len(got) != 0 {
		t.Fatalf("mock-платёж создан в production: %+v", got)
	}

	// Даже если mock-платёж уже есть в Store (например, остался от dev),
	// вебхук отклоняется, и статус не меняется.
	p, err := app.Store.CreatePayment(&repo.Payment{
		StudentID: studentID, CourseID: courseID,
		AmountCents: 10000, Currency: "RUB",
		Provider: "mock", ProviderPaymentID: "mock_prod_without_secret",
		Status: repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	rec = webhookMock(t, router, p.ProviderPaymentID, "succeeded")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("mock-вебхук в production: status=%d body=%s, ждали 503", rec.Code, rec.Body.String())
	}
	if got, _ := app.Store.GetPaymentByProviderID(p.ProviderPaymentID); got == nil || got.Status != repo.PaymentPending {
		t.Fatalf("вебхук изменил платёж: %+v", got)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ открылся через запрещённый mock-вебхук: total=%d", total)
	}
}

// 9. Даже с MOCK_WEBHOOK_SECRET mock в production остаётся запрещён:
// страница подтверждения не умеет подписывать вебхук, поэтому production
// обязан использовать реального провайдера. Проверяем 503 и на покупку,
// и на уже существующий mock-платёж.
func TestPaymentsHardeningMockForbiddenInProductionWithSecret(t *testing.T) {
	const secret = "prod-mock-webhook-secret"
	t.Setenv("MOCK_WEBHOOK_SECRET", secret)

	app, router := newTestAppWithCfg(productionPaymentsCfg("mock"))
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "prod-mock-signed@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("mock в production с секретом: status=%d body=%s, ждали 503", rec.Code, rec.Body.String())
	}
	if got := app.Store.ListPaymentsByStudent(studentID); len(got) != 0 {
		t.Fatalf("mock-платёж создан в production с секретом: %+v", got)
	}

	p, err := app.Store.CreatePayment(&repo.Payment{
		StudentID: studentID, CourseID: courseID,
		AmountCents: 10000, Currency: "RUB",
		Provider: "mock", ProviderPaymentID: "mock_prod_with_secret",
		Status: repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	raw, err := json.Marshal(map[string]any{"providerPaymentId": p.ProviderPaymentID, "status": "succeeded"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec = doRequestHeaders(t, router, http.MethodPost, "/api/v1/payments/webhook/mock",
		json.RawMessage(raw), nil, map[string]string{"X-Mock-Signature": mockSignature(secret, raw)})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("mock-вебхук в production с секретом: status=%d body=%s, ждали 503", rec.Code, rec.Body.String())
	}
	if got, _ := app.Store.GetPaymentByProviderID(p.ProviderPaymentID); got == nil || got.Status != repo.PaymentPending {
		t.Fatalf("вебхук изменил платёж: %+v", got)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ открылся через mock-вебхук в production: total=%d", total)
	}
}

// 10. Вебхук должен принадлежать платежу: mock-вебхук к stripe-платежу — 404,
// статус не меняется, доступ не выдаётся.
func TestPaymentsHardeningWebhookRejectsForeignProvider(t *testing.T) {
	t.Setenv("MOCK_WEBHOOK_SECRET", "")

	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	studentCookies, studentID := registerUser(t, router, "foreign-provider@example.com", "password123", "")

	p, err := app.Store.CreatePayment(&repo.Payment{
		StudentID: studentID, CourseID: courseID,
		AmountCents: 10000, Currency: "RUB",
		Provider: "stripe", ProviderPaymentID: "pi_foreign_provider",
		Status: repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	rec := webhookMock(t, router, p.ProviderPaymentID, "succeeded")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mock-вебхук к stripe-платежу: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
	}
	if got, _ := app.Store.GetPaymentByProviderID(p.ProviderPaymentID); got == nil || got.Status != repo.PaymentPending {
		t.Fatalf("чужой вебхук изменил платёж: %+v", got)
	}
	if e, ok := app.Store.GetEnrollment(studentID, courseID); ok && e.Status == repo.EnrollmentActive {
		t.Fatalf("чужой вебхук открыл доступ: %+v", e)
	}
	if total := myCoursesTotal(t, router, studentCookies); total != 0 {
		t.Fatalf("доступ открылся через чужой вебхук: total=%d", total)
	}
}

// 11. Явная state machine и out-of-order доставки: поздние failed/refunded/
// succeeded не отзывают и не воскрешают доступ, промокод не считается дважды.
func TestPaymentsHardeningOutOfOrderWebhookStateMachine(t *testing.T) {
	t.Setenv("MOCK_WEBHOOK_SECRET", "")

	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	seedPromo(t, router, courseID, "OOO20", 20, 5, teacherCookies)
	studentCookies, studentID := registerUser(t, router, "out-of-order@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock", "promoCode": "OOO20",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	pid, _ := out["providerPaymentId"].(string)
	if pid == "" {
		t.Fatalf("нет providerPaymentId: %s", rec.Body.String())
	}

	webhook := func(status string) string {
		t.Helper()
		r := webhookMock(t, router, pid, status)
		if r.Code != http.StatusOK {
			t.Fatalf("webhook %s: status=%d body=%s", status, r.Code, r.Body.String())
		}
		var body map[string]any
		decodeBody(t, r, &body)
		label, _ := body["status"].(string)
		return label
	}
	paymentStatus := func() repo.PaymentStatus {
		t.Helper()
		p, ok := app.Store.GetPaymentByProviderID(pid)
		if !ok {
			t.Fatalf("платёж %s не найден", pid)
		}
		return p.Status
	}
	enrollmentStatus := func() repo.EnrollmentStatus {
		e, ok := app.Store.GetEnrollment(studentID, courseID)
		if !ok {
			return ""
		}
		return e.Status
	}
	assertState := func(wantPayment repo.PaymentStatus, wantEnrollment repo.EnrollmentStatus, wantUsed int, wantAccess int) {
		t.Helper()
		if got := paymentStatus(); got != wantPayment {
			t.Fatalf("статус платежа=%s, ждали %s", got, wantPayment)
		}
		if got := enrollmentStatus(); got != wantEnrollment {
			t.Fatalf("статус записи=%q, ждали %q", got, wantEnrollment)
		}
		if got := promoByCode(t, app, courseID, "OOO20").UsedCount; got != wantUsed {
			t.Fatalf("usedCount=%d, ждали %d", got, wantUsed)
		}
		if got := myCoursesTotal(t, router, studentCookies); got != wantAccess {
			t.Fatalf("доступных курсов=%d, ждали %d", got, wantAccess)
		}
	}

	// pending/неизвестный статус — ignored, ничего не меняет.
	if got := webhook("pending"); got != "ignored" {
		t.Fatalf("pending: status=%s, ждали ignored", got)
	}
	assertState(repo.PaymentPending, "", 0, 0)

	// pending -> failed: processed; повтор failed — duplicate.
	if got := webhook("failed"); got != "processed" {
		t.Fatalf("failed из pending: status=%s, ждали processed", got)
	}
	assertState(repo.PaymentFailed, "", 0, 0)
	if got := webhook("failed"); got != "duplicate" {
		t.Fatalf("повтор failed: status=%s, ждали duplicate", got)
	}
	assertState(repo.PaymentFailed, "", 0, 0)

	// failed -> succeeded: processed, доступ и промокод выдаются один раз.
	if got := webhook("succeeded"); got != "processed" {
		t.Fatalf("succeeded из failed: status=%s, ждали processed", got)
	}
	assertState(repo.PaymentSucceeded, repo.EnrollmentActive, 1, 1)

	// succeeded -> failed: ignored, доступ не отзывается.
	if got := webhook("failed"); got != "ignored" {
		t.Fatalf("failed после succeeded: status=%s, ждали ignored", got)
	}
	assertState(repo.PaymentSucceeded, repo.EnrollmentActive, 1, 1)

	// succeeded -> refunded: processed, доступ отзывается, usedCount не уменьшается.
	if got := webhook("refunded"); got != "processed" {
		t.Fatalf("refunded из succeeded: status=%s, ждали processed", got)
	}
	assertState(repo.PaymentRefunded, repo.EnrollmentRefunded, 1, 0)

	// refunded -> succeeded: ignored, доступ не воскрешает и промокод не растёт.
	if got := webhook("succeeded"); got != "ignored" {
		t.Fatalf("succeeded после refunded: status=%s, ждали ignored", got)
	}
	assertState(repo.PaymentRefunded, repo.EnrollmentRefunded, 1, 0)

	// refunded -> refunded: duplicate.
	if got := webhook("refunded"); got != "duplicate" {
		t.Fatalf("повтор refunded: status=%s, ждали duplicate", got)
	}
	assertState(repo.PaymentRefunded, repo.EnrollmentRefunded, 1, 0)
}
