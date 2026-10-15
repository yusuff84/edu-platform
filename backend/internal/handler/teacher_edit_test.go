package handler

// Тесты бэклога T06 (T13): GET/PATCH /teacher/courses/{id},
// PATCH /teacher/courses/{id}/modules/{moduleID} и сквозная проводка
// маршрутов T07/T09 (админ-журнал, mock-страница оплаты).

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// sessionCookieFor ставит сессию на уже существующего пользователя.
func sessionCookieFor(t *testing.T, app *App, userID string) *http.Cookie {
	t.Helper()
	sess := app.Store.CreateSession(userID, time.Hour)
	return &http.Cookie{Name: sessionCookie, Value: sess.ID}
}

func withCookie(c *http.Cookie) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(c) }
}

// setupTeacherCourse создаёт курс с модулем и уроком у указанного учителя.
func setupTeacherCourse(t *testing.T, app *App, teacherID string) (*repo.Course, *repo.Module) {
	t.Helper()
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: "test-course-1", Title: "Тест",
		Description: "Описание", PriceCents: 1000,
	})
	m := app.Store.CreateModule(c.ID, "Модуль 1")
	app.Store.CreateLesson(&repo.Lesson{
		ModuleID: m.ID, Title: "Урок 1", Kind: repo.LessonVideoText, BodyMD: "Текст",
	})
	return c, m
}

func TestTeacherGetCourseByID(t *testing.T) {
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, "t-get@x.ru", repo.RoleTeacher)
	cookie := sessionCookieFor(t, app, u.ID)

	c, _ := setupTeacherCourse(t, app, u.ID)

	// 401 без cookie.
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses/"+c.ID, "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("без cookie: status=%d, ждали 401", rec.Code)
	}
	// Свой курс — 200.
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses/"+c.ID, "", withCookie(cookie)); rec.Code != http.StatusOK {
		t.Fatalf("свой курс: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Чужой курс — 403.
	foreign := app.Store.CreateCourse(&repo.Course{
		TeacherID: "другой", Slug: "foreign-course", Title: "Чужой", Description: "x",
	})
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses/"+foreign.ID, "", withCookie(cookie)); rec.Code != http.StatusForbidden {
		t.Fatalf("чужой курс: status=%d, ждали 403", rec.Code)
	}

	rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses/"+c.ID, "", withCookie(cookie))
	var out struct {
		Course  repo.Course `json:"course"`
		Modules []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Lessons []struct {
				ID string `json:"id"`
			} `json:"lessons"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("разбор ответа: %v; body=%s", err, rec.Body.String())
	}
	if out.Course.ID != c.ID || len(out.Modules) != 1 || len(out.Modules[0].Lessons) != 1 {
		t.Fatalf("ожидали курс с 1 модулем и 1 уроком: %s", rec.Body.String())
	}
}

func TestTeacherUpdateCourse(t *testing.T) {
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, "t-patch@x.ru", repo.RoleTeacher)
	cookie := sessionCookieFor(t, app, u.ID)
	c, _ := setupTeacherCourse(t, app, u.ID)

	// Частичное обновление: только title и priceCents.
	rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"title":"Новое имя","priceCents":2500}`, withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updated repo.Course
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if updated.Title != "Новое имя" || updated.PriceCents != 2500 {
		t.Fatalf("поля не обновились: %+v", updated)
	}
	if updated.Description != "Описание" {
		t.Fatalf("описание не должно меняться при частичном PATCH: %q", updated.Description)
	}

	// Пустое тело — 400 (нужно хотя бы одно поле).
	if rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID, `{}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой PATCH: status=%d, ждали 400", rec.Code)
	}
	// Отрицательная цена — 400.
	if rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"priceCents":-1}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("отрицательная цена: status=%d, ждали 400", rec.Code)
	}
	// Неизвестное поле — 400 (decodeJSON с DisallowUnknownFields).
	if rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"status":"published"}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("статус через PATCH: status=%d, ждали 400 (модерация — через submit)", rec.Code)
	}
	// Занятый slug — 400.
	app.Store.CreateCourse(&repo.Course{TeacherID: u.ID, Slug: "taken-slug-1", Title: "x", Description: "y"})
	if rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"slug":"taken-slug-1"}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("занятый slug: status=%d, ждали 400", rec.Code)
	}
	// Свой slug — не занят, PATCH проходит.
	if rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"slug":"my-own-slug"}`, withCookie(cookie)); rec.Code != http.StatusOK {
		t.Fatalf("свой slug: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTeacherUpdateModule(t *testing.T) {
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, "t-mod@x.ru", repo.RoleTeacher)
	cookie := sessionCookieFor(t, app, u.ID)
	c, m := setupTeacherCourse(t, app, u.ID)

	rec := doJSON(t, router, http.MethodPatch,
		"/api/v1/teacher/courses/"+c.ID+"/modules/"+m.ID,
		`{"title":"Переименованный"}`, withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mod repo.Module
	if err := json.Unmarshal(rec.Body.Bytes(), &mod); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if mod.Title != "Переименованный" || mod.ID != m.ID {
		t.Fatalf("модуль не переименован: %+v", mod)
	}
	// Счётчик модулей не вырос — PATCH не создаёт новый.
	if n := len(app.Store.ListModulesByCourse(c.ID)); n != 1 {
		t.Fatalf("PATCH не должен создавать модуль, есть %d", n)
	}

	// Пустой title — 400.
	if rec := doJSON(t, router, http.MethodPatch,
		"/api/v1/teacher/courses/"+c.ID+"/modules/"+m.ID,
		`{"title":"  "}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой title: status=%d, ждали 400", rec.Code)
	}
	// Чужой модуль (не принадлежит курсу) — 404.
	foreign := app.Store.CreateCourse(&repo.Course{TeacherID: u.ID, Slug: "foreign-mod-1", Title: "x", Description: "y"})
	fm := app.Store.CreateModule(foreign.ID, "Чужой модуль")
	if rec := doJSON(t, router, http.MethodPatch,
		"/api/v1/teacher/courses/"+c.ID+"/modules/"+fm.ID,
		`{"title":"Хак"}`, withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("чужой модуль: status=%d, ждали 404", rec.Code)
	}
}

// TestWiringAdminPayments — сквозная проверка T07: маршрут подключён,
// без админской cookie отвечает 401, с админской — 200.
func TestWiringAdminPayments(t *testing.T) {
	app, router := newTestRouter(devConfig())
	// Без cookie — 401 (маршрут вообще зарегистрирован, не 404).
	rec := doJSON(t, router, http.MethodGet, "/api/v1/admin/payments", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("admin/payments без cookie: status=%d, ждали 401", rec.Code)
	}
	// Не-админ — 403.
	sc := sessionCookieFor(t, app, mustUser(t, app, "student@x.ru", repo.RoleStudent).ID)
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/admin/payments", "", withCookie(sc)); rec.Code != http.StatusForbidden {
		t.Fatalf("admin/payments не-админом: status=%d, ждали 403", rec.Code)
	}
	// Админ — 200.
	ac := sessionCookieFor(t, app, mustUser(t, app, "admin@x.ru", repo.RoleAdmin).ID)
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/admin/payments", "", withCookie(ac)); rec.Code != http.StatusOK {
		t.Fatalf("admin/payments админом: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Старые пути не задублированы: /api/v1/admin/admin/payments не существует.
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/admin/admin/payments", "", withCookie(ac)); rec.Code != http.StatusNotFound {
		t.Fatalf("дубль /admin/admin/payments не должен существовать: status=%d", rec.Code)
	}
	// settings тоже подключены.
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/admin/settings", "", withCookie(ac)); rec.Code != http.StatusOK {
		t.Fatalf("admin/settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// mustUser создаёт пользователя или падает.
func mustUser(t *testing.T, app *App, email string, role repo.Role) *repo.User {
	t.Helper()
	u, err := app.Store.CreateUser(email, "хеш", role)
	if err != nil {
		t.Fatalf("создание %s: %v", email, err)
	}
	return u
}

// TestWiringMockPayPage — сквозная проверка T09: /pay/mock/{id} на верхнем
// уровне роутера отдаёт HTML-страницу, а не 404.
func TestWiringMockPayPage(t *testing.T) {
	_, router := newTestRouter(devConfig())
	rec := doJSON(t, router, http.MethodGet, "/pay/mock/mock_abc123", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("mock-страница: status=%d, ждали 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("mock-страница должна быть HTML, got %q", ct)
	}
}
