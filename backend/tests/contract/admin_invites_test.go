package contract_test

// Контрактные тесты T47: список приглашений (GET /admin/invites) и отзыв
// (POST /admin/invites/{id}/revoke) — формы из Task/00-api-contract.md
// («Эндпоинты волны P6»): элемент без token/tokenHash, но с tokenPrefix;
// статус — сырой статус хранилища; revoke только из sent (200/404/409).

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/mailer"
	"edu-platform/backend/internal/repo"
)

// findInviteByEmail достаёт элемент списка по email (или nil).
func findInviteByEmail(items []any, email string) map[string]any {
	for _, raw := range items {
		row, _ := raw.(map[string]any)
		if row["email"] == email {
			return row
		}
	}
	return nil
}

// Список и отзыв приглашений: создание → запись в списке (без токена,
// с tokenPrefix), пагинация limit=1&page=2, revoke → 200 revoked,
// повторный revoke → 409, случайный UUID → 404, гварды ролей 403/401.
func TestAdminInvitesListAndRevoke(t *testing.T) {
	app, router := newTestApp()
	adminCookies, adminID := registerUser(t, router, "invites-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "invites-admin@example.com")

	// Пустой список: items — пустой массив (не null), total=0, дефолты страницы.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/invites", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("пустой список: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	if items, ok := list["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("items должен быть пустым массивом: %s", rec.Body.String())
	}
	if total, _ := list["total"].(float64); total != 0 {
		t.Fatalf("total=%v на старте", list["total"])
	}
	if page, _ := list["page"].(float64); page != 1 {
		t.Fatalf("page по умолчанию=%v, ждали 1", list["page"])
	}
	if limit, _ := list["limit"].(float64); limit != 20 {
		t.Fatalf("limit по умолчанию=%v, ждали 20", list["limit"])
	}

	// Два приглашения: каждое создание — 201 с одноразовым токеном.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "invite-one@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite one: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var createdOne map[string]any
	decodeBody(t, rec, &createdOne)
	idOne, _ := createdOne["id"].(string)
	prefixOne, _ := createdOne["tokenPrefix"].(string)
	if idOne == "" || prefixOne == "" {
		t.Fatalf("ответ создания приглашения неполон: %s", rec.Body.String())
	}

	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "invite-two@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite two: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var createdTwo map[string]any
	decodeBody(t, rec, &createdTwo)
	idTwo, _ := createdTwo["id"].(string)
	if idTwo == "" || idTwo == idOne {
		t.Fatalf("id второго приглашения неверен: %s", rec.Body.String())
	}

	// Полный список: 2 записи, новые первыми; элемент — форма контракта,
	// без token/tokenHash, статус — сырой "sent".
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("список: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &list)
	if total, _ := list["total"].(float64); total != 2 {
		t.Fatalf("total=%v, ждали 2: %s", list["total"], rec.Body.String())
	}
	items, _ := list["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("в списке %d записей, ждали 2: %s", len(items), rec.Body.String())
	}
	first, _ := items[0].(map[string]any)
	if first["id"] != idTwo {
		t.Fatalf("сортировка должна быть createdAt DESC (второе приглашение первым): %s", rec.Body.String())
	}
	// Форма элемента: все поля контракта на месте.
	if _, hasToken := first["token"]; hasToken {
		t.Fatalf("в списке не должно быть token: %s", rec.Body.String())
	}
	if _, hasHash := first["tokenHash"]; hasHash {
		t.Fatalf("в списке не должно быть tokenHash: %s", rec.Body.String())
	}
	if first["status"] != "sent" {
		t.Fatalf("status=%v, ждали sent", first["status"])
	}
	if prefix, _ := first["tokenPrefix"].(string); prefix == "" {
		t.Fatalf("tokenPrefix пуст: %s", rec.Body.String())
	}
	for _, field := range []string{"expiresAt", "createdAt"} {
		if s, _ := first[field].(string); s == "" {
			t.Fatalf("%s пуст: %s", field, rec.Body.String())
		}
	}
	if first["createdBy"] != adminID {
		t.Fatalf("createdBy=%v, ждали id админа %s", first["createdBy"], adminID)
	}

	// Пагинация: limit=1 → только новейшее; page=2 → только первое.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites?limit=1", nil, adminCookies)
	decodeBody(t, rec, &list)
	paged, _ := list["items"].([]any)
	if len(paged) != 1 || paged[0].(map[string]any)["id"] != idTwo {
		t.Fatalf("limit=1: ждали одно новейшее приглашение: %s", rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites?limit=1&page=2", nil, adminCookies)
	decodeBody(t, rec, &list)
	if page, _ := list["page"].(float64); page != 2 {
		t.Fatalf("page=%v, ждали 2", list["page"])
	}
	if total, _ := list["total"].(float64); total != 2 {
		t.Fatalf("total на второй странице=%v (должен быть прежним)", list["total"])
	}
	paged, _ = list["items"].([]any)
	if len(paged) != 1 || paged[0].(map[string]any)["id"] != idOne {
		t.Fatalf("page=2&limit=1: ждали старшее приглашение: %s", rec.Body.String())
	}
	// limit > 100 клампится; нечисловые page/limit → 400.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites?limit=500", nil, adminCookies)
	decodeBody(t, rec, &list)
	if limit, _ := list["limit"].(float64); limit != 100 {
		t.Fatalf("limit=%v, ждали clamp до 100", list["limit"])
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites?limit=abc", nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit=abc: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Параметры page и limit должны быть целыми числами" {
		t.Fatalf("текст ошибки page/limit: %v", errBody["error"])
	}

	// Отзыв: 200 с {"invite": {... status revoked}}.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites/"+idOne+"/revoke", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var revoked map[string]any
	decodeBody(t, rec, &revoked)
	inv, _ := revoked["invite"].(map[string]any)
	if inv == nil || inv["id"] != idOne || inv["status"] != "revoked" {
		t.Fatalf("ответ revoke: %s", rec.Body.String())
	}
	if prefix, _ := inv["tokenPrefix"].(string); prefix == "" {
		t.Fatalf("элемент revoke без tokenPrefix: %s", rec.Body.String())
	}

	// Статус изменился и в списке.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites", nil, adminCookies)
	decodeBody(t, rec, &list)
	items, _ = list["items"].([]any)
	if row := findInviteByEmail(items, "invite-one@example.com"); row == nil || row["status"] != "revoked" {
		t.Fatalf("после revoke статус в списке: %s", rec.Body.String())
	}

	// Повторный отзыв → 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites/"+idOne+"/revoke", nil, adminCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("повторный revoke: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Отозвать можно только активное приглашение" {
		t.Fatalf("текст 409: %v", errBody["error"])
	}

	// Несуществующее приглашение → 404.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/admin/invites/00000000-0000-0000-0000-000000000000/revoke", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoke несуществующего: status=%d body=%s, ждали 404", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Приглашение не найдено" {
		t.Fatalf("текст 404: %v", errBody["error"])
	}

	// Гварды ролей: студент и учитель → 403; аноним → 401.
	plainStudentCookies, _ := registerUser(t, router, "invites-student@example.com", "password123", "")
	registerUser(t, router, "invites-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "invites-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	teacherCookies := loginUser(t, router, "invites-teacher@example.com", "password123")

	for _, c := range []struct {
		name    string
		cookies []*http.Cookie
	}{
		{"студент", plainStudentCookies},
		{"учитель", teacherCookies},
	} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites", nil, c.cookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s на GET /admin/invites: status=%d body=%s, ждали 403", c.name, rec.Code, rec.Body.String())
		}
		rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites/"+idTwo+"/revoke", nil, c.cookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s на revoke: status=%d body=%s, ждали 403", c.name, rec.Code, rec.Body.String())
		}
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/invites", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на списке: status=%d, ждали 401", rec.Code)
	}
}

// captureInviteMailer — двойник SMTP: запоминает письма и умеет вернуть
// ошибку, чтобы проверить, что приглашение не теряется при сбое доставки.
type captureInviteMailer struct {
	messages []mailer.Message
	err      error
}

func (m *captureInviteMailer) Send(_ context.Context, msg mailer.Message) error {
	m.messages = append(m.messages, msg)
	return m.err
}

// При настроенном Mailer приглашение уходит письмом; ссылка и emailSent
// возвращаются в API. Ошибка SMTP не отменяет приглашение: 201, emailSent=false.
func TestAdminInviteEmailDelivery(t *testing.T) {
	app, router := newTestApp()
	app.Config.PublicAppURL = "https://edu.example/"
	adminCookies, _ := registerUser(t, router, "invite-mail-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "invite-mail-admin@example.com")

	fake := &captureInviteMailer{}
	app.Mailer = fake

	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "mail-teacher@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	decodeBody(t, rec, &created)
	token, _ := created["token"].(string)
	link, _ := created["inviteLink"].(string)
	if token == "" || !strings.HasPrefix(link, "https://edu.example/register?inviteToken="+token) {
		t.Fatalf("ответ без корректной ссылки: %s", rec.Body.String())
	}
	if sent, _ := created["emailSent"].(bool); !sent {
		t.Fatalf("emailSent=%v, ждали true: %s", created["emailSent"], rec.Body.String())
	}
	if len(fake.messages) != 1 {
		t.Fatalf("писем отправлено %d, ждали 1", len(fake.messages))
	}
	msg := fake.messages[0]
	if msg.To != "mail-teacher@example.com" ||
		!strings.Contains(msg.Subject, "Приглашение") ||
		!strings.Contains(msg.Text, link) {
		t.Fatalf("письмо не соответствует приглашению: %+v", msg)
	}

	// Сбой SMTP: приглашение остаётся созданным, админ получает ссылку.
	fake.err = errors.New("smtp недоступен")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "mail-teacher-2@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite при сбое SMTP: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var failed map[string]any
	decodeBody(t, rec, &failed)
	if sent, _ := failed["emailSent"].(bool); sent {
		t.Fatalf("emailSent=true при ошибке SMTP: %s", rec.Body.String())
	}
	if link, _ := failed["inviteLink"].(string); !strings.Contains(link, "inviteToken=") {
		t.Fatalf("при сбое SMTP не отдана резервная ссылка: %s", rec.Body.String())
	}
}
