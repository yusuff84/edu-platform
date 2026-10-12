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
	store := repo.NewStore()
	store.SetCommissionPct(15)
	app := &handler.App{Store: store, Config: cfg}
	return app, handler.NewRouter(app)
}

func doRequest(t *testing.T, router http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
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
