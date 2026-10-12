package contract_test

import (
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// seedPublishedCourse: учитель -> черновик -> модуль+quiz-урок -> submit -> approve.
// Возвращает cookie учителя, id и slug курса.
func seedPublishedCourse(t *testing.T, app *handler.App, router http.Handler, priceCents int) (teacherCookies []*http.Cookie, courseID, slug string) {
	t.Helper()

	adminCookies, _ := registerUser(t, router, "admin@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "admin@example.com"), repo.RoleAdmin) {
		t.Fatal("не удалось выдать роль admin")
	}
	// Перелогин не обязателен: роль читается из store при каждом запросе.

	teacherCookies, _ = registerUser(t, router, "teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	// Черновик.
	var course map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Go для новичков", "description": "Базовый курс", "priceCents": priceCents,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &course)
	courseID, _ = course["id"].(string)
	slug, _ = course["slug"].(string)

	// Пустой курс на модерацию — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("submit пустого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Модуль + quiz-урок.
	var mod map[string]any
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Основы",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &mod)
	moduleID, _ := mod["id"].(string)

	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": moduleID, "title": "Переменные", "kind": "quiz",
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "q1", "text": "Что такое переменная?", "options": []string{"a", "b"}, "correct": 0,
		}}},
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Submit -> review.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Очередь модерации видна админу.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/review", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("review queue: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Approve.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/moderate", map[string]any{
		"decision": "approve",
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return teacherCookies, courseID, slug
}

func userIDByEmail(t *testing.T, store repo.StoreAPI, email string) string {
	t.Helper()
	u, ok := store.GetUserByEmail(email)
	if !ok {
		t.Fatalf("пользователь %s не найден", email)
	}
	return u.ID
}

// Каталог показывает только published; страница курса скрывает ответы квиза.
func TestCoursesCatalogAndPage(t *testing.T) {
	app, router := newTestApp()
	_, courseID, slug := seedPublishedCourse(t, app, router, 50000)
	_ = courseID

	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var catalog map[string]any
	decodeBody(t, rec, &catalog)
	if total, _ := catalog["total"].(float64); total != 1 {
		t.Fatalf("catalog total=%v, body=%s", catalog["total"], rec.Body.String())
	}

	// Поиск.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses?q=go", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Страница курса: правильных ответов быть не должно.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"correct"`) {
		t.Fatalf("страница курса раскрывает правильные ответы: %s", rec.Body.String())
	}

	// Несуществующий slug — 404.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/no-such-course", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing course: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Студент не может создавать курсы — 403.
	studentCookies, _ := registerUser(t, router, "s@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "X", "description": "Y",
	}, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("student create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
