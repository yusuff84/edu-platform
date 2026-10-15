package handler

// Тесты безопасности аутентификации (T10): rate-limit, подпись cookie,
// Secure/SameSite-флаги и жёсткий CSRF в прод-режиме.
// Инжектируемое время (rateLimiter.now) делает проверку ведра детерминированной.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// newTestRouter поднимает роутер поверх in-memory store.
func newTestRouter(cfg *config.Config) (*App, http.Handler) {
	app := &App{Store: repo.NewStore(), Config: cfg}
	return app, NewRouter(app)
}

// devConfig — dev-конфиг как в контрактных тестах: лимиты выключены (<=0),
// подпись cookie отключена, Origin-заголовки не требуются.
func devConfig() *config.Config {
	return &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		RunnerURL:        "http://127.0.0.1:1",
		AllowedOrigins:   []string{"http://localhost:3000"},
	}
}

// prodConfig — прод-конфиг: DATABASE_URL задан, APP_ENV=production,
// значит строгий CSRF и обязательный SESSION_SECRET.
func prodConfig() *config.Config {
	cfg := devConfig()
	cfg.AppEnv = "production"
	cfg.DatabaseURL = "postgres://edu:edu@db:5432/edu"
	cfg.SessionSecret = "секрет-для-теста"
	return cfg
}

// doJSON шлёт JSON-запрос; mod опционально меняет запрос (заголовки, cookie).
func doJSON(t *testing.T, router http.Handler, method, path, body string, mod func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func sessionCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatalf("в ответе нет cookie %q; Set-Cookie: %v", sessionCookie, rec.Header().Values("Set-Cookie"))
	return nil
}

// TestRateLimiterBucket — детерминированная проверка token bucket на фиктивных часах.
func TestRateLimiterBucket(t *testing.T) {
	now := time.Now()
	l := newRateLimiter(3, time.Minute, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("ip-a"); !ok {
			t.Fatalf("запрос %d должен пройти", i+1)
		}
	}
	ok, wait := l.allow("ip-a")
	if ok {
		t.Fatal("4-й запрос подряд должен быть отклонён")
	}
	if wait <= 0 || wait > time.Minute {
		t.Fatalf("Retry-After вне ожидаемого диапазона: %v", wait)
	}

	// Другой IP — свой бюджет.
	if ok, _ := l.allow("ip-b"); !ok {
		t.Fatal("лимит не должен делиться между IP")
	}

	// Через 20 секунд возвращается ровно один токен (3 токена / 60 сек).
	now = now.Add(20 * time.Second)
	if ok, _ := l.allow("ip-a"); !ok {
		t.Fatal("после пополнения запрос должен пройти")
	}
	if ok, _ := l.allow("ip-a"); ok {
		t.Fatal("второй запрос сразу после пополнения должен быть отклонён")
	}

	// Cleanup удаляет протухшие ведра — карта не течёт по памяти.
	now = now.Add(3 * time.Minute)
	l.cleanup()
	l.mu.Lock()
	left := len(l.buckets)
	l.mu.Unlock()
	if left != 0 {
		t.Fatalf("после cleanup должно остаться 0 ведёр, есть %d", left)
	}
}

// TestRateLimitHTTP429 — HTTP-уровень: login ограничен, 429 + Retry-After + русский error.
func TestRateLimitHTTP429(t *testing.T) {
	cfg := devConfig()
	cfg.RateLimitAuthPerMin = 3
	_, router := newTestRouter(cfg)

	body := `{"email":"nobody@example.com","password":"password1"}`
	for i := 0; i < 3; i++ {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", body, nil)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("запрос %d не должен попадать под лимит", i+1)
		}
	}
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", body, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4-й запрос должен получить 429, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("нет заголовка Retry-After при 429")
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["error"] == "" {
		t.Fatalf("нет envelope {error}: %s", rec.Body.String())
	}
	if !strings.Contains(out["error"], "Слишком много запросов") {
		t.Fatalf("текст ошибки не русский: %q", out["error"])
	}
}

// TestRateLimitDisabledByNonPositiveConfig — RateLimitAuthPerMin <= 0
// (dev-поведение, конфиг по умолчанию) полностью выключает лимитер:
// много запросов подряд без 429. Регресс: случайная активация лимитера
// в dev сломала бы сквозные прогоны и тесты.
func TestRateLimitDisabledByNonPositiveConfig(t *testing.T) {
	for _, limit := range []int{0, -1, -100} {
		cfg := devConfig()
		cfg.RateLimitAuthPerMin = limit
		_, router := newTestRouter(cfg)

		body := `{"email":"nobody@example.com","password":"password1"}`
		for i := 0; i < 20; i++ {
			rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", body, nil)
			if rec.Code == http.StatusTooManyRequests {
				t.Fatalf("limit=%d: запрос %d не должен попадать под лимит", limit, i+1)
			}
		}
	}
}

// TestSessionCookieSignature — cookie = <id>.<подпись>, подделка → 401.
func TestSessionCookieSignature(t *testing.T) {
	cfg := devConfig()
	cfg.SessionSecret = "супер-секрет-тест"
	_, router := newTestRouter(cfg)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"sig@example.com","password":"password123"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := sessionCookieOf(t, rec)

	id, sig, ok := strings.Cut(cookie.Value, ".")
	if !ok || id == "" || sig == "" {
		t.Fatalf("cookie = «%s», ожидали <sessionID>.<подпись>", cookie.Value)
	}
	if want := signSessionID(cfg.SessionSecret, id); sig != want {
		t.Fatalf("подпись неверная: got %s, want %s", sig, want)
	}
	if !cookie.HttpOnly {
		t.Fatal("cookie не httpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, ожидали Lax", cookie.SameSite)
	}
	if cookie.Secure {
		t.Fatal("на HTTP-запросе Secure не должен выставляться")
	}

	// Валидная cookie — 200; displayName присутствует (контракт API).
	me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", func(r *http.Request) { r.AddCookie(cookie) })
	if me.Code != http.StatusOK {
		t.Fatalf("me: status=%d body=%s", me.Code, me.Body.String())
	}
	var user map[string]any
	if err := json.Unmarshal(me.Body.Bytes(), &user); err != nil {
		t.Fatalf("me: %v", err)
	}
	if _, ok := user["displayName"]; !ok {
		t.Fatalf("в ответе нет displayName (контракт API): %s", me.Body.String())
	}

	// 1. Подделанная подпись — 401.
	forged := &http.Cookie{Name: sessionCookie, Value: id + ".подделанная-подпись"}
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", func(r *http.Request) { r.AddCookie(forged) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("подделанная подпись: status=%d, ждали 401", rec.Code)
	}

	// 2. Голый sessionID без подписи — 401 (секрет задан, подпись обязательна).
	bare := &http.Cookie{Name: sessionCookie, Value: id}
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", func(r *http.Request) { r.AddCookie(bare) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("голый sessionID: status=%d, ждали 401", rec.Code)
	}

	// 3. Подпись чужого sessionID, склеенная с другим id, — 401.
	alien := &http.Cookie{Name: sessionCookie, Value: "00000000-0000-0000-0000-000000000000." + sig}
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", func(r *http.Request) { r.AddCookie(alien) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("чужой id + подпись: status=%d, ждали 401", rec.Code)
	}

	// Logout с валидной cookie чистит сессию (и разбирает подписанный формат).
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/logout", "{}", func(r *http.Request) { r.AddCookie(cookie) }); rec.Code != http.StatusOK {
		t.Fatalf("logout: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", func(r *http.Request) { r.AddCookie(cookie) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me после logout: status=%d, ждали 401", rec.Code)
	}
}

// TestDevWithoutSecretBareCookie — dev без SESSION_SECRET: cookie голая (совместимость),
// прод с пустым секретом не стартует (проверяется в config.Load).
func TestDevWithoutSecretBareCookie(t *testing.T) {
	_, router := newTestRouter(devConfig())
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"bare@example.com","password":"password123"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := sessionCookieOf(t, rec)
	if strings.Contains(cookie.Value, ".") {
		t.Fatalf("в dev без секрета cookie не должна подписываться: %s", cookie.Value)
	}
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", func(r *http.Request) { r.AddCookie(cookie) }); rec.Code != http.StatusOK {
		t.Fatalf("me: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSecureAndSameSiteFlags — Secure по X-Forwarded-Proto: https; SameSite из конфига.
func TestSecureAndSameSiteFlags(t *testing.T) {
	cfg := devConfig()
	cfg.SessionSecret = "секрет"
	cfg.CookieSameSite = http.SameSiteStrictMode
	_, router := newTestRouter(cfg)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"https@example.com","password":"password123"}`,
		func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") })
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := sessionCookieOf(t, rec)
	if !cookie.Secure {
		t.Fatal("за HTTPS cookie должна быть Secure")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, ожидали Strict из конфига", cookie.SameSite)
	}
}

// TestCSRFProdStrict — в проде меняющие запросы без Origin/Referer отклоняются,
// webhook-и платежек и «безопасные» запросы исключены.
func TestCSRFProdStrict(t *testing.T) {
	_, router := newTestRouter(prodConfig())

	// 1. POST без Origin/Referer — 403.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"noorigin@example.com","password":"password123"}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("register без Origin: status=%d, ждали 403", rec.Code)
	}

	// 2. Разрешённый Origin — проходит.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"good@example.com","password":"password123"}`,
		func(r *http.Request) { r.Header.Set("Origin", "http://localhost:3000") })
	if rec.Code != http.StatusCreated {
		t.Fatalf("register с Origin: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 3. Чужой Origin — 403.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"good@example.com","password":"password123"}`,
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login с чужим Origin: status=%d, ждали 403", rec.Code)
	}

	// 4. Bearer-заголовок без Origin — «заведомо безопасный», не 403 (уйдёт в хендлер).
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"good@example.com","password":"password123"}`,
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer token") })
	if rec.Code == http.StatusForbidden {
		t.Fatalf("запрос с bearer не должен отклоняться CSRF: status=%d", rec.Code)
	}

	// 5. Webhook платежки без Origin не блокируется (его шлют серверы провайдеров).
	rec = doJSON(t, router, http.MethodPost, "/api/v1/payments/webhook/mock",
		`{"providerPaymentId":"nope","status":"succeeded"}`, nil)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("webhook не должен попадать под CSRF-проверку: status=%d", rec.Code)
	}
}

// TestCSRFAllowMissingOriginFlag — env-флаг возвращает dev-поведение в prod.
func TestCSRFAllowMissingOriginFlag(t *testing.T) {
	cfg := prodConfig()
	cfg.CSRFAllowMissingOrigin = true
	_, router := newTestRouter(cfg)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"script@example.com","password":"password123"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register без Origin при CSRF_ALLOW_MISSING_ORIGIN: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestDecodeJSONFriendlyErrors — понятные русские ошибки вместо «json: unknown field».
func TestDecodeJSONFriendlyErrors(t *testing.T) {
	_, router := newTestRouter(devConfig())

	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"a@b.co","password":"password123","лишнее":1}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, ждали 400", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("нет envelope: %v", err)
	}
	if !strings.Contains(out["error"], "неизвестное поле") {
		t.Fatalf("ожидали подсказку про неизвестное поле, got %q", out["error"])
	}

	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":123,"password":"password123"}`, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("нет envelope: %v", err)
	}
	if !strings.Contains(out["error"], "неверный тип") {
		t.Fatalf("ожидали подсказку про тип поля, got %q", out["error"])
	}
}
