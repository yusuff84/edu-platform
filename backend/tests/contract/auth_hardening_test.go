package contract_test

// Контрактные тесты ужесточения аутентификации (T10) на границе API:
// подпись cookie, влияние SESSION_SECRET, rate-limit 429, строгий CSRF
// в прод-режиме, CORS-заголовки, лимит тела 1 МБ и пагинация.
// Юнит-уровень этих механизмов покрыт internal/handler/security_test.go;
// здесь проверяется то же самое через полный HTTP-роутер.

import (
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/config"
)

// prodCSRFConfig — прод-конфиг для контрактных тестов: жёсткий CSRF
// (только Origin из allowlist), подпись cookie обязательна.
func prodCSRFConfig() *config.Config {
	return &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		AppEnv:           "production",
		SessionSecret:    "контрактный-секрет-т12",
		AllowedOrigins:   []string{"http://localhost:3000"},
	}
}

// Cookie подписана: подмена последнего символа → 401.
func TestContractCookieTamperRejected(t *testing.T) {
	cfg := prodCSRFConfig()
	_, router := newTestAppWithCfg(cfg)

	rec := doRequestHeaders(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "tamper@example.com", "password": "password123",
	}, nil, map[string]string{"Origin": "http://localhost:3000"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := cookieByName(rec, "session_id")
	if cookie == nil || !strings.Contains(cookie.Value, ".") {
		t.Fatalf("cookie не подписана (ожидали <id>.<подпись>): %v", cookie)
	}

	// Подмена последнего символа подписи.
	last := cookie.Value[len(cookie.Value)-1]
	flipped := "A"
	if last == 'A' {
		flipped = "B"
	}
	tampered := &http.Cookie{Name: "session_id", Value: cookie.Value[:len(cookie.Value)-1] + flipped}

	// Оригинал работает...
	rec = doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("оригинальная cookie: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// ...подделка — 401.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{tampered})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("подделка cookie: status=%d, ждали 401", rec.Code)
	}
}

// SESSION_SECRET реально влияет на cookie: выданный одним секретом
// не проходит на сервере с другим секретом.
func TestContractSessionSecretAffectsValue(t *testing.T) {
	cfgA := prodCSRFConfig()
	cfgB := prodCSRFConfig()
	cfgB.SessionSecret = "другой-секрет"
	_, routerA := newTestAppWithCfg(cfgA)
	_, routerB := newTestAppWithCfg(cfgB)

	rec := doRequestHeaders(t, routerA, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "secret@example.com", "password": "password123",
	}, nil, map[string]string{"Origin": "http://localhost:3000"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := cookieByName(rec, "session_id")
	if cookie == nil {
		t.Fatal("нет cookie")
	}

	rec = doRequest(t, routerA, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("свой секрет: status=%d", rec.Code)
	}
	rec = doRequest(t, routerB, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{cookie})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("чужой секрет: status=%d, ждали 401 (подпись не совпала)", rec.Code)
	}
}

// Rate-limit: N+1-й логин подряд → 429 + Retry-After; лимиты двух серверов
// независимы (лимитер живёт в App, а не глобально — тесты не зависят от порядка).
func TestContractRateLimitLogin429(t *testing.T) {
	cfg := &config.Config{
		PaymentsProvider:   "mock",
		CommissionPct:      15,
		AllowedOrigins:     []string{},
		RateLimitAuthPerMin: 3,
	}
	_, router := newTestAppWithCfg(cfg)

	body := map[string]any{"email": "nobody@example.com", "password": "password1"}
	for i := 0; i < 3; i++ {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/login", body, nil)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("запрос %d не должен попадать под лимит", i+1)
		}
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/login", body, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4-й логин: status=%d, ждали 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("нет Retry-After при 429")
	}
	var err429 map[string]any
	decodeBody(t, rec, &err429)
	if msg, _ := err429["error"].(string); !strings.Contains(msg, "Слишком много запросов") {
		t.Fatalf("текст 429 не русский: %v", err429["error"])
	}

	// Второй сервер с тем же конфигом имеет собственный бюджет.
	_, router2 := newTestAppWithCfg(cfg)
	rec = doRequest(t, router2, http.MethodPost, "/api/v1/auth/login", body, nil)
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("лимит не должен делиться между экземплярами приложения")
	}
}

// Строгий CSRF в прод-режиме: меняющие запросы требуют Origin из allowlist,
// webhook-и платежек исключены.
func TestContractCSRFStrictProd(t *testing.T) {
	_, router := newTestAppWithCfg(prodCSRFConfig())

	// Без Origin — 403.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "no-origin@example.com", "password": "password123",
	}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("register без Origin: status=%d, ждали 403", rec.Code)
	}

	// Чужой Origin — 403.
	rec = doRequestHeaders(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "evil@example.com", "password": "password123",
	}, nil, map[string]string{"Origin": "https://evil.example"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой Origin: status=%d, ждали 403", rec.Code)
	}

	// Разрешённый Origin — работает.
	rec = doRequestHeaders(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "ok-origin@example.com", "password": "password123",
	}, nil, map[string]string{"Origin": "http://localhost:3000"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("allowed Origin: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Webhook без Origin не блокируется CSRF (его шлют серверы провайдеров).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": "нет-такого", "status": "succeeded",
	}, nil)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("webhook не должен попадать под CSRF: status=%d", rec.Code)
	}
}

// CORS: чужой Origin не получает Access-Control-Allow-Origin, разрешённый —
// получает ровно свой origin и Allow-Credentials.
func TestContractCORSHeaders(t *testing.T) {
	_, router := newTestAppWithCfg(prodCSRFConfig())

	rec := doRequestHeaders(t, router, http.MethodGet, "/api/v1/courses", nil, nil,
		map[string]string{"Origin": "https://evil.example"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("чужой Origin получил ACAO=%q", got)
	}

	rec = doRequestHeaders(t, router, http.MethodGet, "/api/v1/courses", nil, nil,
		map[string]string{"Origin": "http://localhost:3000"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("ACAO=%q, ждали ровно origin", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("нет Allow-Credentials для разрешённого origin")
	}
}

// Лимит тела запроса 1 МБ: переполненное тело → 400, а не 201/500.
func TestContractBodyLimit1MB(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, _ := registerUser(t, router, "bigbody@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "bigbody@example.com"), "teacher") {
		t.Fatal("не удалось выдать роль teacher")
	}

	// Тело > 1 МБ обрезается LimitReader'ом → JSON становится невалидным → 400.
	huge := strings.Repeat("а", (1<<20)+1024)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Огромный", "description": huge,
	}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("тело >1МБ: status=%d, ждали 400", rec.Code)
	}

	// Контроль: маленькое тело того же эндпоинта проходит.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Нормальный", "description": "короткое описание",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("нормальное тело: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Пагинация каталога: limit > 100 обрезается до 100, page=0 → первая страница.
func TestContractPaginationClamp(t *testing.T) {
	_, router := newTestApp()

	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses?limit=500", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var page map[string]any
	decodeBody(t, rec, &page)
	if limit, _ := page["limit"].(float64); limit != 100 {
		t.Fatalf("limit=%v, ждали clamp до 100", page["limit"])
	}

	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses?page=0", nil, nil)
	decodeBody(t, rec, &page)
	if p, _ := page["page"].(float64); p != 1 {
		t.Fatalf("page=0 должен стать 1, got %v", page["page"])
	}
}

// OAuth Яндекса без YANDEX_CLIENT_ID — 503 с русским текстом (не 404).
func TestContractYandexNotConfigured503(t *testing.T) {
	_, router := newTestApp() // в конфиге нет YandexClientID

	rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/yandex/start", nil, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("yandex/start: status=%d, ждали 503", rec.Code)
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if msg, _ := errBody["error"].(string); !strings.Contains(msg, "не настроен") {
		t.Fatalf("текст ошибки OAuth не русский/нет подсказки: %v", errBody["error"])
	}
}

// Фактическое поведение (расхождение №2 контракта): /auth/logout возвращает
// 200 {"status":"ok"}, а не 204. Контракт обещает 204 — фронтенд принимает
// оба варианта; тест фиксирует факт, чтобы изменение было заметным.
func TestContractLogoutActualBehavior(t *testing.T) {
	_, router := newTestApp()
	cookies, _ := registerUser(t, router, "logout@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/logout", map[string]any{}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: status=%d, фактическое поведение — 200 (расхождение №2)", rec.Code)
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["status"] != "ok" {
		t.Fatalf("тело logout: %s", rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, cookies)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me после logout: status=%d", rec.Code)
	}
}

// Фактическое поведение (расхождение №5 контракта): /healthz возвращает
// JSON {"status":"ok"}, а не текст ok. Сверяется с docs/API.md.
func TestContractHealthzActualBehavior(t *testing.T) {
	_, router := newTestApp()

	for _, path := range []string{"/healthz", "/api/v1/healthz"} {
		rec := doRequest(t, router, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d", path, rec.Code)
		}
		var body map[string]any
		decodeBody(t, rec, &body)
		if body["status"] != "ok" {
			t.Fatalf("%s: тело %q — фактически JSON {\"status\":\"ok\"} (расхождение №5)", path, rec.Body.String())
		}
	}
}
