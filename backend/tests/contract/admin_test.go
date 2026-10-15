package contract_test

// Контрактные тесты админ-функций: очередь модерации, решения,
// приглашения учителей, гварды ролей на /admin/*.

import (
	"net/http"
	"testing"

	"edu-platform/backend/internal/repo"
)

// Модерация: очередь показывает только review; reject без комментария — 400;
// reject с комментарием — draft + moderationComment; approve — published;
// повторная модерация — 400.
func TestAdminModerationFlow(t *testing.T) {
	app, router := newTestApp()

	adminCookies, _ := registerUser(t, router, "mod-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "mod-admin@example.com")
	teacherCookies, _ := registerUser(t, router, "mod-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "mod-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	// Черновик A → модуль + урок → submit (review).
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Курс на модерацию", "description": "Описание", "priceCents": 1000,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course A: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var courseA map[string]any
	decodeBody(t, rec, &courseA)
	idA, _ := courseA["id"].(string)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+idA+"/modules", map[string]any{
		"title": "Модуль",
	}, teacherCookies)
	var modA map[string]any
	decodeBody(t, rec, &modA)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+idA+"/lessons", map[string]any{
		"moduleId": modA["id"], "title": "Урок", "kind": "video_text", "bodyMd": "текст",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson A: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+idA+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit A: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Черновик B остаётся draft и в очередь не попадает.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Черновик B", "description": "Описание", "priceCents": 0,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course B: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var courseB map[string]any
	decodeBody(t, rec, &courseB)

	// Очередь: только курс A.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/review", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("queue: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var queue map[string]any
	decodeBody(t, rec, &queue)
	items, _ := queue["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("в очереди %d курсов, ждали 1: %s", len(items), rec.Body.String())
	}
	if items[0].(map[string]any)["id"] != idA {
		t.Fatalf("в очереди чужой курс: %s", rec.Body.String())
	}

	// Reject без комментария — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+idA+"/moderate", map[string]any{
		"decision": "reject",
	}, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reject без комментария: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Reject с комментарием — draft + moderationComment.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+idA+"/moderate", map[string]any{
		"decision": "reject", "comment": "Слишком мало материалов",
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rejected map[string]any
	decodeBody(t, rec, &rejected)
	if rejected["status"] != "draft" {
		t.Fatalf("после reject status=%v, ждали draft", rejected["status"])
	}
	if mc, _ := rejected["moderationComment"].(string); mc == "" {
		t.Fatalf("moderationComment пуст: %s", rec.Body.String())
	}

	// Повторная модерация черновика — 400 (не на модерации).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+idA+"/moderate", map[string]any{
		"decision": "approve",
	}, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("модерация черновика: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Teacher resubmits, admin approve → published.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+idA+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("resubmit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+idA+"/moderate", map[string]any{
		"decision": "approve",
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var approved map[string]any
	decodeBody(t, rec, &approved)
	if approved["status"] != "published" {
		t.Fatalf("после approve status=%v, ждали published", approved["status"])
	}

	// Опубликованный курс виден анониму, черновик B — нет.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses", nil, nil)
	var catalog map[string]any
	decodeBody(t, rec, &catalog)
	if total, _ := catalog["total"].(float64); total != 1 {
		t.Fatalf("каталог total=%v, ждали 1 (только approved)", catalog["total"])
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseB["slug"].(string), nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("черновик виден анониму: status=%d", rec.Code)
	}
}

// Гварды ролей: студент получает 403 на всех /admin/* эндпоинтах
// (модерация, приглашения, журнал платежей, настройки).
func TestAdminRoleGuards(t *testing.T) {
	app, router := newTestApp()
	studentCookies, _ := registerUser(t, router, "admin-guard@example.com", "password123", "")

	checks := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/courses/review"},
		{http.MethodPost, "/api/v1/admin/invites"},
		{http.MethodGet, "/api/v1/admin/payments"},
		{http.MethodGet, "/api/v1/admin/settings"},
		{http.MethodPut, "/api/v1/admin/settings"},
	}
	for _, c := range checks {
		rec := doRequest(t, router, c.method, c.path, map[string]any{"email": "x@y.co", "commissionPct": 15, "decision": "approve"}, studentCookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("студент на %s %s: status=%d body=%s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	// Без входа — 401 (не 403): requireAuth срабатывает раньше requireRole.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/payments", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на /admin/payments: status=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = app
}

// Приглашение: POST /admin/invites выдаёт одноразовый токен,
// работоспособный в регистрации (проверка сквозного цикла invite→teacher).
func TestAdminInviteWorksInRegistration(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := registerUser(t, router, "invite-root@example.com", "password123", "")
	promoteToAdmin(t, app, "invite-root@example.com")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "invited-teacher@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var inv map[string]any
	decodeBody(t, rec, &inv)
	token, _ := inv["token"].(string)
	if token == "" {
		t.Fatal("в ответе нет токена")
	}

	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "invited-teacher@example.com", "password": "password123", "inviteToken": token,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register с invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var u map[string]any
	decodeBody(t, rec, &u)
	if u["role"] != "teacher" {
		t.Fatalf("роль=%v, ждали teacher", u["role"])
	}

	// Невалидный токен — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "ghost@example.com", "password": "password123", "inviteToken": "мусор",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("мусорный invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
