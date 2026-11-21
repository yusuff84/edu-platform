package contract_test

// Заявки «Поможем подобрать курс»: POST /leads сохраняет контакт,
// валидирует поля, молча отбрасывает ботов (поле-ловушка website);
// GET /admin/leads и POST /admin/leads/{id}/status — только админу.

import (
	"net/http"
	"testing"
)

func TestLeadsCreateListAndStatus(t *testing.T) {
	app, router := newTestApp()

	valid := map[string]any{"name": "Анна", "phone": "+7 (999) 123-45-67", "email": "Anna@Example.com", "contactMethod": "telegram", "consent": true}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/leads", valid, nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST /leads: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Бот заполнил ловушку: 201, но в базе ничего.
	bot := map[string]any{"name": "Bot", "phone": "+79990000000", "consent": true, "website": "http://spam"}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/leads", bot, nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST /leads (bot): status=%d", rec.Code)
	}

	bad := []map[string]any{
		{"name": "А", "phone": "+79991234567", "consent": true},
		{"name": "Анна", "phone": "12", "consent": true},
		{"name": "Анна", "phone": "+79991234567", "consent": false},
		{"name": "Анна", "phone": "+79991234567", "email": "nope", "consent": true},
		{"name": "Анна", "phone": "+79991234567", "contactMethod": "fax", "consent": true},
	}
	for i, b := range bad {
		if rec := doRequest(t, router, http.MethodPost, "/api/v1/leads", b, nil); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad[%d]: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// Без входа и не-админу список закрыт.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/leads", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /admin/leads anon: status=%d", rec.Code)
	}
	student, _ := registerUser(t, router, "lead-student@example.test", "password123", "")
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/leads", nil, student); rec.Code != http.StatusForbidden {
		t.Fatalf("GET /admin/leads student: status=%d", rec.Code)
	}

	registerUser(t, router, "lead-admin@example.test", "password123", "")
	promoteToAdmin(t, app, "lead-admin@example.test")
	admin := loginUser(t, router, "lead-admin@example.test", "password123")
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/leads", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/leads: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []struct {
			ID, Name, Phone, Email, ContactMethod, Status string
		} `json:"items"`
		NewCount int `json:"newCount"`
	}
	decodeBody(t, rec, &out)
	if len(out.Items) != 1 || out.NewCount != 1 {
		t.Fatalf("ожидалась 1 новая заявка, получено %+v", out)
	}
	l := out.Items[0]
	if l.Name != "Анна" || l.Phone != "+79991234567" || l.Email != "anna@example.com" || l.ContactMethod != "telegram" || l.Status != "new" {
		t.Fatalf("неверные поля заявки: %+v", l)
	}

	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/leads/"+l.ID+"/status", map[string]string{"status": "done"}, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("set status: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/leads/00000000-0000-0000-0000-000000000000/status", map[string]string{"status": "done"}, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("set status unknown: %d", rec.Code)
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/leads", nil, admin)
	decodeBody(t, rec, &out)
	if out.NewCount != 0 || out.Items[0].Status != "done" {
		t.Fatalf("статус не обновился: %+v", out)
	}
}
