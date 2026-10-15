package contract_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

func newTestApp() (*handler.App, http.Handler) {
	cfg := &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		RunnerURL:        "http://127.0.0.1:1", // недоступен: code-уроки должны вернуть 502
		AllowedOrigins:   []string{},
	}
	return newTestAppWithCfg(cfg)
}

// newTestAppWithCfg собирает приложение поверх in-memory store с произвольным
// конфигом: нужно для тестов CSRF (prod-режим), rate-limit и подписи cookie.
func newTestAppWithCfg(cfg *config.Config) (*handler.App, http.Handler) {
	store := repo.NewStore()
	if cfg.PaymentsProvider == "" {
		cfg.PaymentsProvider = "mock"
	}
	if cfg.RunnerURL == "" {
		cfg.RunnerURL = "http://127.0.0.1:1"
	}
	if cfg.CommissionPct > 0 {
		store.SetCommissionPct(cfg.CommissionPct)
	}
	app := &handler.App{Store: store, Config: cfg}
	return app, handler.NewRouter(app)
}

func doRequest(t *testing.T, router http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	return doRequestHeaders(t, router, method, path, body, cookies, nil)
}

// doRequestHeaders — doRequest с дополнительными заголовками (Origin и т.п.).
func doRequestHeaders(t *testing.T, router http.Handler, method, path string, body any, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

// registerUser регистрирует пользователя и возвращает его cookie и id.
func registerUser(t *testing.T, router http.Handler, email, password, inviteToken string) (cookies []*http.Cookie, id string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": email, "password": password, "inviteToken": inviteToken,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	id, _ = out["id"].(string)
	return rec.Result().Cookies(), id
}

func loginUser(t *testing.T, router http.Handler, email, password string) []*http.Cookie {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": email, "password": password,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

// promoteToAdmin выдаёт роль admin напрямую в store: в проде нет API смены
// ролей (StoreAPI.SetUserRole не вызывается ни одним хендлером), поэтому
// админа в тестах заводим только так.
func promoteToAdmin(t *testing.T, app *handler.App, email string) {
	t.Helper()
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, email), repo.RoleAdmin) {
		t.Fatalf("не удалось выдать роль admin %s", email)
	}
}

// cookieByName возвращает cookie с нужным именем из ответа (или nil).
func cookieByName(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
