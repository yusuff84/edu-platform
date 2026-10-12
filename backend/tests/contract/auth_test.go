package contract_test

import (
	"net/http"
	"testing"
)

// register -> login, ошибки валидации и неверный пароль.
func TestAuthRegisterLogin(t *testing.T) {
	_, router := newTestApp()

	cookies, _ := registerUser(t, router, "student@example.com", "password123", "")

	// /auth/me с cookie — 200.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Дублирующий email — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "student@example.com", "password": "password123",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate register: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Неверный пароль — 401.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "student@example.com", "password": "wrongpass1",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Верный логин — 200 + cookie.
	cookies = loginUser(t, router, "student@example.com", "password123")
	if len(cookies) == 0 {
		t.Fatal("login не вернул cookie")
	}

	// Logout чистит сессию: me после logout — 401.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/logout", map[string]any{}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, cookies)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me после logout: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthValidation(t *testing.T) {
	_, router := newTestApp()

	cases := []map[string]any{
		{"email": "not-an-email", "password": "password123"},
		{"email": "a@b.co", "password": "short"},
		{"email": "", "password": "password123"},
	}
	for i, body := range cases {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/register", body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("case %d: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
		var out map[string]any
		decodeBody(t, rec, &out)
		if _, ok := out["error"]; !ok {
			t.Fatalf("case %d: нет envelope {error}", i)
		}
	}

	// Невалидный inviteToken — 400.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "t@example.com", "password": "password123", "inviteToken": "мусор",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
