package contract_test

// Новая админка (handler/admin_hub.go): журнал действий администраторов,
// сводка и сквозные списки — только для админа.

import (
	"net/http"
	"strings"
	"testing"
)

func TestAdminHub(t *testing.T) {
	app, router := newTestApp()
	registerUser(t, router, "hub-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "hub-admin@example.com")
	admin := loginUser(t, router, "hub-admin@example.com", "password123")
	studentCookies, _ := registerUser(t, router, "hub-student@example.com", "password123", "")

	for _, p := range []string{"/api/v1/admin/overview", "/api/v1/admin/audit", "/api/v1/admin/questions", "/api/v1/admin/reviews", "/api/v1/admin/promo-codes"} {
		if rec := doRequest(t, router, http.MethodGet, p, nil, studentCookies); rec.Code != http.StatusForbidden {
			t.Fatalf("student %s: %d", p, rec.Code)
		}
		if rec := doRequest(t, router, http.MethodGet, p, nil, admin); rec.Code != http.StatusOK {
			t.Fatalf("admin %s: %d %s", p, rec.Code, rec.Body.String())
		}
	}

	// Сводка: 30 дней по умолчанию, регистрации сегодня учтены.
	var ov struct {
		Attention map[string]int `json:"attention"`
		Series    []struct {
			Date    string `json:"date"`
			Signups int    `json:"signups"`
		} `json:"series"`
		RecentUsers []struct{ Email string } `json:"recentUsers"`
	}
	decodeBody(t, doRequest(t, router, http.MethodGet, "/api/v1/admin/overview", nil, admin), &ov)
	if len(ov.Series) != 30 || ov.Series[29].Signups < 2 || len(ov.RecentUsers) < 2 {
		t.Fatalf("overview: %+v", ov)
	}
	if _, ok := ov.Attention["unansweredQuestions"]; !ok {
		t.Fatalf("attention: %+v", ov.Attention)
	}

	// Блокировка ученика попадает в журнал с email и понятной подписью.
	var users struct {
		Items []struct{ ID, Email string } `json:"items"`
	}
	decodeBody(t, doRequest(t, router, http.MethodGet, "/api/v1/admin/users?q=hub-student", nil, admin), &users)
	if len(users.Items) != 1 {
		t.Fatalf("users: %+v", users)
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+users.Items[0].ID+"/status", map[string]any{"status": "blocked"}, admin); rec.Code != http.StatusOK {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}
	// Действие с явной подписью (auditAdmin) — одна запись, без дублей.
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "hub-new@example.com", "role": "student"}, admin); rec.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body.String())
	}
	// Неудачное действие в журнал не попадает.
	doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/00000000-0000-0000-0000-000000000000/status", map[string]any{"status": "blocked"}, admin)

	var audit struct {
		Items []struct {
			Action     string `json:"action"`
			ActorEmail string `json:"actorEmail"`
		} `json:"items"`
		Total int `json:"total"`
	}
	decodeBody(t, doRequest(t, router, http.MethodGet, "/api/v1/admin/audit", nil, admin), &audit)
	if audit.Total != 2 {
		t.Fatalf("audit total = %d: %+v", audit.Total, audit.Items)
	}
	if !strings.Contains(audit.Items[0].Action, "Создан пользователь hub-new@example.com") ||
		!strings.Contains(audit.Items[1].Action, "hub-student@example.com заблокирован") ||
		audit.Items[0].ActorEmail != "hub-admin@example.com" {
		t.Fatalf("audit items: %+v", audit.Items)
	}
	decodeBody(t, doRequest(t, router, http.MethodGet, "/api/v1/admin/audit?q=заблокирован", nil, admin), &audit)
	if audit.Total != 1 {
		t.Fatalf("audit search: %+v", audit)
	}

	// Действия не-админов в журнал не пишутся (ученик меняет свой профиль и т.п.).
	doRequest(t, router, http.MethodPost, "/api/v1/auth/logout", nil, studentCookies)
	decodeBody(t, doRequest(t, router, http.MethodGet, "/api/v1/admin/audit", nil, admin), &audit)
	if audit.Total != 2 {
		t.Fatalf("audit after student action: %d", audit.Total)
	}
}
