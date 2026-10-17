package contract_test

// Контрактные тесты цикла преподавателя (specs T028).
// Роли, черновик, валидация уроков по kind, модерация guard'ы,
// endpoints редактирования от T13 и подтверждённые расхождения контракта.

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// inviteToken в регистрации даёт роль teacher; без токена — student.
// Неверный/чужой/повторно использованный токен — 400.
func TestTeacherRegistrationRoles(t *testing.T) {
	app, router := newTestApp()

	// Админ заводит приглашение.
	adminCookies, _ := registerUser(t, router, "invite-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "invite-admin@example.com")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "new-teacher@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var inv map[string]any
	decodeBody(t, rec, &inv)
	token, _ := inv["token"].(string)
	if token == "" || inv["tokenPrefix"] == "" || inv["expiresAt"] == "" {
		t.Fatalf("форма приглашения нарушена: %s", rec.Body.String())
	}

	// Регистрация с токеном — роль teacher (и поле displayName есть: расхождение №1 закрыто).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "new-teacher@example.com", "password": "password123", "inviteToken": token,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register с invite: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var teacher map[string]any
	decodeBody(t, rec, &teacher)
	if teacher["role"] != "teacher" {
		t.Fatalf("роль=%v, ждали teacher", teacher["role"])
	}
	if _, ok := teacher["displayName"]; !ok {
		t.Fatalf("нет displayName в ответе (расхождение №1 закрыто T10): %s", rec.Body.String())
	}

	// Без токена — student.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "plain-student@example.com", "password": "password123",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var student map[string]any
	decodeBody(t, rec, &student)
	if student["role"] != "student" {
		t.Fatalf("роль=%v, ждали student", student["role"])
	}

	// Повторное использование токена — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "another@example.com", "password": "password123", "inviteToken": token,
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("повторный invite: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Токен для чужого email — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/invites", map[string]any{
		"email": "second@example.com",
	}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite 2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var inv2 map[string]any
	decodeBody(t, rec, &inv2)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": "not-me@example.com", "password": "password123", "inviteToken": inv2["token"],
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invite для чужого email: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Полный цикл: не-учитель — 403, автосlug, submit пустого курса — 400,
// валидация уроков по kind, submit с содержимым — review.
func TestTeacherCourseLifecycle(t *testing.T) {
	app, router := newTestApp()

	// Студент не видит /teacher/* — 403.
	studentCookies, _ := registerUser(t, router, "not-teacher@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("student на /teacher/courses: status=%d body=%s", rec.Code, rec.Body.String())
	}

	teacherCookies, _ := registerUser(t, router, "lifecycle-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "lifecycle-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	// Создание курса: 201, автосlug course-xxxxxxxx.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Тестовый курс", "description": "Описание", "priceCents": 5000,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var course map[string]any
	decodeBody(t, rec, &course)
	courseID, _ := course["id"].(string)
	slug, _ := course["slug"].(string)
	if !regexp.MustCompile(`^course-[0-9a-f]{8}$`).MatchString(slug) {
		t.Fatalf("автосlug не вида course-xxxxxxxx: %q", slug)
	}
	if course["status"] != "draft" {
		t.Fatalf("новый курс должен быть draft: %v", course["status"])
	}

	// Список своих курсов — конверт {items,total}.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("my courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	if total, _ := list["total"].(float64); total != 1 {
		t.Fatalf("total=%v body=%s", list["total"], rec.Body.String())
	}

	// Submit пустого курса — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("submit пустого: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Модуль.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Модуль 1",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mod map[string]any
	decodeBody(t, rec, &mod)
	moduleID, _ := mod["id"].(string)

	// Чужой курс — 403 при добавлении модуля.
	otherCookies, _ := registerUser(t, router, "other-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "other-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Чужой модуль",
	}, otherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой курс: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Валидация уроков по kind: каждый случай — 400.
	invalidLessons := []map[string]any{
		// quiz без вопросов.
		{"moduleId": moduleID, "title": "Пустой квиз", "kind": "quiz",
			"quiz": map[string]any{"questions": []any{}}},
		// quiz с correct вне диапазона.
		{"moduleId": moduleID, "title": "Плохой индекс", "kind": "quiz",
			"quiz": map[string]any{"questions": []any{map[string]any{
				"id": "q1", "text": "Вопрос?", "options": []string{"a", "b"}, "correct": 5,
			}}}},
		// code без тестов.
		{"moduleId": moduleID, "title": "Код без тестов", "kind": "code",
			"codeTask": map[string]any{"language": "python", "template": "", "tests": []any{}}},
		// несуществующий moduleId.
		{"moduleId": "00000000-0000-0000-0000-000000000000", "title": "Нет модуля", "kind": "video_text", "bodyMd": "текст"},
		// неизвестный kind.
		{"moduleId": moduleID, "title": "Магия", "kind": "magic", "bodyMd": "текст"},
	}
	for i, body := range invalidLessons {
		rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", body, teacherCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid lesson #%d: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// Валидный quiz-урок — 201.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": moduleID, "title": "Квиз", "kind": "quiz",
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "q1", "text": "2+2?", "options": []string{"3", "4"}, "correct": 1,
		}}},
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Submit → review.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var submitted map[string]any
	decodeBody(t, rec, &submitted)
	if submitted["status"] != "review" {
		t.Fatalf("status=%v, ждали review", submitted["status"])
	}

	// Повторный submit (уже на модерации) — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("повторный submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Endpoints редактирования от T13: GET/PATCH /teacher/courses/{id},
// PATCH .../modules/{moduleID}. Студент — 403.
func TestTeacherEditEndpoints(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	// GET по id: конверт {course, modules} с уроками.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID, nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("get course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var detail map[string]any
	decodeBody(t, rec, &detail)
	c, ok := detail["course"].(map[string]any)
	if !ok || c["id"] != courseID {
		t.Fatalf("нет course в конверте: %s", rec.Body.String())
	}
	mods, _ := detail["modules"].([]any)
	if len(mods) == 0 {
		t.Fatalf("нет modules: %s", rec.Body.String())
	}

	// PATCH курса: смена названия.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+courseID, map[string]any{
		"title": "Новое название",
	}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var upd map[string]any
	decodeBody(t, rec, &upd)
	if upd["title"] != "Новое название" {
		t.Fatalf("название не обновилось: %s", rec.Body.String())
	}
	// Пустой PATCH — 400.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+courseID, map[string]any{}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой patch: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// PATCH модуля: переименование.
	mod := mods[0].(map[string]any)
	rec = doRequest(t, router, http.MethodPatch,
		"/api/v1/teacher/courses/"+courseID+"/modules/"+mod["id"].(string),
		map[string]any{"title": "Переименованный модуль"}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updMod map[string]any
	decodeBody(t, rec, &updMod)
	if updMod["title"] != "Переименованный модуль" {
		t.Fatalf("модуль не переименован: %s", rec.Body.String())
	}

	// Студент — 403 на все три.
	studentCookies, _ := registerUser(t, router, "edit-student@example.com", "password123", "")
	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/teacher/courses/" + courseID},
		{http.MethodPatch, "/api/v1/teacher/courses/" + courseID},
		{http.MethodPatch, "/api/v1/teacher/courses/" + courseID + "/modules/" + mod["id"].(string)},
	} {
		rec = doRequest(t, router, req.method, req.path, map[string]any{"title": "x"}, studentCookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s для студента: status=%d", req.method, req.path, rec.Code)
		}
	}
}

// Правильные ответы квиза не утекают анониму через GET /courses/{slug}
// (как в courses_test.go, но для курса, собранного здесь же).
func TestQuizCorrectAnswersDoNotLeak(t *testing.T) {
	app, router := newTestApp()
	_, _, slug := seedPublishedCourse(t, app, router, 0)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"correct"`) {
		t.Fatalf("аноним видит правильные ответы: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"questions"`) {
		t.Fatalf("аноним не видит сам квиз: %s", rec.Body.String())
	}
}

// Расхождение №4 (T16) ЗАКРЫТО: POST /teacher/courses/{id}/lessons сохраняет
// startsAt — планирование живых эфиров работает. Раньше здесь жил тест-фиксатор
// TestStartsAtNotPersistedDiscrepancy4, ожидавший startsAt=null: он упал бы при
// починке бага, поэтому проверка перевёрнута.
func TestStartsAtPersisted(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	mods := app.Store.ListModulesByCourse(courseID)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mods[0].ID, "title": "Эфир", "kind": "live",
		"liveUrl": "https://live.example/stream", "startsAt": "2030-01-01T12:00:00Z",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create live lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	if lesson["startsAt"] != "2030-01-01T12:00:00Z" {
		t.Fatalf("startsAt должен сохраниться и вернуться в ответе: got %v, body=%s",
			lesson["startsAt"], rec.Body.String())
	}
	if lesson["liveUrl"] == "" {
		t.Fatalf("liveUrl должен сохраниться: %s", rec.Body.String())
	}
	// Проверяем, что поле дошло именно до хранилища (баг жил на шаге
	// копирования req.StartsAt → repo.Lesson), а не было просто эхом ответа.
	id, _ := lesson["id"].(string)
	stored, ok := app.Store.GetLesson(id)
	if !ok || stored.StartsAt == nil || stored.StartsAt.Format(time.RFC3339) != "2030-01-01T12:00:00Z" {
		t.Fatalf("startsAt не сохранён в repo.Lesson: %+v", stored)
	}

	// Некорректная дата — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mods[0].ID, "title": "Эфир 2", "kind": "live",
		"liveUrl": "https://live.example/stream", "startsAt": "когда-нибудь",
	}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("некорректный startsAt: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
