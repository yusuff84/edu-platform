package contract_test

// Контрактные тесты T87 — редактирование курса супер-админом.
//
// Проверяется замороженный раздел T87 Task/00-api-contract.md:
//   - admin меняет содержимое published/review-курса через существующие
//     /teacher/courses/{id} и вложенные CRUD-маршруты (урок, модуль, создание,
//     удаление, reorder, перенос урока);
//   - для teacher draft-guard не изменился: published/review → 409;
//   - архивный курс — 409 и для admin (до unarchive);
//   - POST /admin/courses/{id}/publish: draft|review → published,
//     published — идемпотентный 200, archived — 409, нет курса — 404,
//     teacher — 403, аноним — 401, moderationComment очищается;
//   - GET /admin/courses/{id} отдаёт аддитивное course.moderationComment.
//
// Стиль — как у teacher_lifecycle_test.go / admin_course_detail_p7_test.go:
// сквозь NewRouter, фикстуры через API, проверка кодов и тел контракта.

import (
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// t87Env — окружение теста T87: приложение, роутер и готовые сессии
// админа и teacher-автора.
type t87Env struct {
	app     *handler.App
	router  http.Handler
	admin   []*http.Cookie
	teacher []*http.Cookie
}

func t87NewEnv(t *testing.T) *t87Env {
	t.Helper()
	app, router := newTestApp()
	teacher, _ := lifecycleTeacher(t, app, router, "t87-teacher@example.com")
	admin, _ := registerUser(t, router, "t87-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "t87-admin@example.com")
	return &t87Env{app: app, router: router, admin: admin, teacher: teacher}
}

// t87AddModule создаёт модуль от имени cookies и возвращает его id.
func t87AddModule(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID, title string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{"title": title}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("создание модуля %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("в ответе создания модуля нет id: %s", rec.Body.String())
	}
	return id
}

// t87AddLesson создаёт video_text-урок от имени cookies и возвращает его id.
func t87AddLesson(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID, moduleID, title string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
			"moduleId": moduleID, "title": title, "kind": "video_text", "bodyMd": "текст урока",
		}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("создание урока %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("в ответе создания урока нет id: %s", rec.Body.String())
	}
	return id
}

// t87Submit отправляет курс на модерацию (draft → review).
func t87Submit(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit курса %s: status=%d body=%s", courseID, rec.Code, rec.Body.String())
	}
}

// t87PublishOK вызывает force publish и возвращает course из конверта.
func t87PublishOK(t *testing.T, router http.Handler, admin []*http.Cookie, courseID string) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/publish", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish курса %s: status=%d body=%s", courseID, rec.Code, rec.Body.String())
	}
	var out struct {
		Course map[string]any `json:"course"`
	}
	decodeBody(t, rec, &out)
	if out.Course == nil {
		t.Fatalf("publish без конверта {course}: %s", rec.Body.String())
	}
	return out.Course
}

// ---- Сценарий 1: admin правит published-курс ----

func TestAdminCourseEditPublished(t *testing.T) {
	env := t87NewEnv(t)
	courseID := createDraftCourse(t, env.router, env.teacher, "T87 опубликованный")
	m1 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 1")
	l1 := t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 1")
	l2 := t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 2")
	m2 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 2")
	t87Submit(t, env.router, env.teacher, courseID)
	t87PublishOK(t, env.router, env.admin, courseID)

	// PATCH урока опубликованного курса — 200.
	rec := doRequest(t, env.router, http.MethodPatch,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+l1,
		map[string]any{"title": "Урок 1 (admin)"}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PATCH урока published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	if lesson["title"] != "Урок 1 (admin)" {
		t.Fatalf("урок не обновился: %s", rec.Body.String())
	}

	// PATCH модуля опубликованного курса — 200.
	rec = doRequest(t, env.router, http.MethodPatch,
		"/api/v1/teacher/courses/"+courseID+"/modules/"+m1,
		map[string]any{"title": "Модуль 1 (admin)"}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PATCH модуля published: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Создание модуля и урока в published-курсе — 201.
	m3 := t87AddModule(t, env.router, env.admin, courseID, "Модуль 3")
	l3 := t87AddLesson(t, env.router, env.admin, courseID, m3, "Урок 3")

	// Reorder модулей: полный список, m2 → 1, m1 → 2, m3 → 3.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/modules/reorder", map[string]any{
			"items": []any{
				map[string]any{"id": m1, "position": 2},
				map[string]any{"id": m2, "position": 1},
				map[string]any{"id": m3, "position": 3},
			},
		}, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin reorder модулей published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	mods := env.app.Store.ListModulesByCourse(courseID)
	if len(mods) != 3 || mods[0].ID != m2 || mods[1].ID != m1 || mods[2].ID != m3 {
		t.Fatalf("порядок модулей после admin reorder неверен: %+v", mods)
	}

	// Reorder уроков: полный список курса; в m1 l2 → 1, l1 → 2, в m3 l3 → 1.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons/reorder", map[string]any{
			"items": []any{
				map[string]any{"id": l1, "position": 2},
				map[string]any{"id": l2, "position": 1},
				map[string]any{"id": l3, "position": 1},
			},
		}, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin reorder уроков published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	lessonsM1 := env.app.Store.ListLessonsByModule(m1)
	if len(lessonsM1) != 2 || lessonsM1[0].ID != l2 || lessonsM1[1].ID != l1 {
		t.Fatalf("порядок уроков после admin reorder неверен: %+v", lessonsM1)
	}

	// Перенос урока m1 → m3 в published-курсе — 200.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+l1+"/move",
		map[string]any{"moduleId": m3}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin move урока published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if stored, _ := env.app.Store.GetLesson(l1); stored.ModuleID != m3 {
		t.Fatalf("урок не перенесён: %+v", stored)
	}

	// Удаление урока и пустого модуля — 204.
	rec = doRequest(t, env.router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+l2, nil, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin DELETE урока published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/modules/"+m2, nil, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin DELETE модуля published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, found := env.app.Store.GetLesson(l2); found {
		t.Fatal("урок не удалён")
	}
	if _, found := env.app.Store.GetModule(m2); found {
		t.Fatal("модуль не удалён")
	}

	// Статус курса не изменился: правки админа не сбрасывают публикацию.
	if c, ok := env.app.Store.GetCourseByID(courseID); !ok || c.Status != repo.CoursePublished {
		t.Fatalf("статус после правок admin: %+v", c)
	}
}

// ---- Сценарий 2: admin правит review-курс ----

func TestAdminCourseEditReview(t *testing.T) {
	env := t87NewEnv(t)
	courseID := createDraftCourse(t, env.router, env.teacher, "T87 на модерации")
	m1 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 1")
	l1 := t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 1")
	m2 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 2")
	l2 := t87AddLesson(t, env.router, env.teacher, courseID, m2, "Урок 2")
	t87Submit(t, env.router, env.teacher, courseID)

	// PATCH урока и модуля на review — 200.
	rec := doRequest(t, env.router, http.MethodPatch,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+l1,
		map[string]any{"title": "Урок 1 (review)", "bodyMd": "новый текст"}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PATCH урока review: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPatch,
		"/api/v1/teacher/courses/"+courseID+"/modules/"+m1,
		map[string]any{"title": "Модуль 1 (review)"}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PATCH модуля review: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Создание модуля и урока на review — 201.
	m3 := t87AddModule(t, env.router, env.admin, courseID, "Модуль 3")
	l3 := t87AddLesson(t, env.router, env.admin, courseID, m3, "Урок 3")

	// Reorder модулей (полный список) — 204.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/modules/reorder", map[string]any{
			"items": []any{
				map[string]any{"id": m3, "position": 1},
				map[string]any{"id": m1, "position": 2},
				map[string]any{"id": m2, "position": 3},
			},
		}, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin reorder модулей review: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Перенос урока: m1 → m2, затем reorder уроков — 200/204.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+l1+"/move",
		map[string]any{"moduleId": m2}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin move урока review: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons/reorder", map[string]any{
			"items": []any{
				map[string]any{"id": l1, "position": 1},
				map[string]any{"id": l2, "position": 2},
				map[string]any{"id": l3, "position": 1},
			},
		}, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin reorder уроков review: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Удаление урока и опустевшего модуля — 204.
	rec = doRequest(t, env.router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/lessons/"+l1, nil, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin DELETE урока review: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/modules/"+m1, nil, env.admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin DELETE модуля review: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Статус остался review: админ не публикует и не сбрасывает курс.
	if c, ok := env.app.Store.GetCourseByID(courseID); !ok || c.Status != repo.CourseReview {
		t.Fatalf("статус после правок admin в review: %+v", c)
	}
}

// ---- Сценарий 3: для teacher draft-guard не изменился ----

func TestTeacherContentGuardsPreserved(t *testing.T) {
	env := t87NewEnv(t)

	// Два курса с одинаковой программой: published и review.
	build := func(title string, published bool) (courseID, m1, m2, l1, l2 string) {
		t.Helper()
		courseID = createDraftCourse(t, env.router, env.teacher, title)
		m1 = t87AddModule(t, env.router, env.teacher, courseID, "Модуль 1")
		l1 = t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 1")
		m2 = t87AddModule(t, env.router, env.teacher, courseID, "Модуль 2")
		l2 = t87AddLesson(t, env.router, env.teacher, courseID, m2, "Урок 2")
		t87Submit(t, env.router, env.teacher, courseID)
		if published {
			t87PublishOK(t, env.router, env.admin, courseID)
		}
		return courseID, m1, m2, l1, l2
	}

	pubID, pubM1, pubM2, pubL1, pubL2 := build("T87 teacher published", true)
	revID, revM1, revM2, revL1, revL2 := build("T87 teacher review", false)
	cases := []struct {
		name             string
		courseID, m1, m2 string
		l1, l2           string
	}{
		{"published", pubID, pubM1, pubM2, pubL1, pubL2},
		{"review", revID, revM1, revM2, revL1, revL2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := "/api/v1/teacher/courses/" + tc.courseID

			// PATCH урока — 409 с прежним текстом T19.
			rec := doRequest(t, env.router, http.MethodPatch, base+"/lessons/"+tc.l1,
				map[string]any{"title": "teacher не должен"}, env.teacher)
			if rec.Code != http.StatusConflict ||
				!strings.Contains(rec.Body.String(), "Контент курса можно редактировать только в режиме черновика") {
				t.Fatalf("teacher PATCH урока %s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}

			// Создание модуля/урока у teacher draft-guard не имело и не имеет
			// (T87 «правила не меняются»), поэтому здесь его не проверяем.

			// Удаление урока/модуля — 409.
			rec = doRequest(t, env.router, http.MethodDelete, base+"/lessons/"+tc.l2, nil, env.teacher)
			if rec.Code != http.StatusConflict {
				t.Fatalf("teacher DELETE урока %s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}
			rec = doRequest(t, env.router, http.MethodDelete, base+"/modules/"+tc.m2, nil, env.teacher)
			if rec.Code != http.StatusConflict {
				t.Fatalf("teacher DELETE модуля %s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}

			// Reorder модулей/уроков — 409 (guard раньше валидации тела).
			rec = doRequest(t, env.router, http.MethodPost, base+"/modules/reorder", map[string]any{
				"items": []any{
					map[string]any{"id": tc.m1, "position": 2},
					map[string]any{"id": tc.m2, "position": 1},
				},
			}, env.teacher)
			if rec.Code != http.StatusConflict {
				t.Fatalf("teacher reorder модулей %s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}
			rec = doRequest(t, env.router, http.MethodPost, base+"/lessons/reorder", map[string]any{
				"items": []any{
					map[string]any{"id": tc.l1, "position": 1},
					map[string]any{"id": tc.l2, "position": 1},
				},
			}, env.teacher)
			if rec.Code != http.StatusConflict {
				t.Fatalf("teacher reorder уроков %s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}

			// Перенос урока — 409 с прежним текстом T21.
			rec = doRequest(t, env.router, http.MethodPost, base+"/lessons/"+tc.l1+"/move",
				map[string]any{"moduleId": tc.m2}, env.teacher)
			if rec.Code != http.StatusConflict ||
				!strings.Contains(rec.Body.String(), "Менять структуру можно только в черновике") {
				t.Fatalf("teacher move урока %s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

// ---- Сценарий 4: архивный курс — 409 и для admin ----

func TestAdminCourseEditArchived(t *testing.T) {
	env := t87NewEnv(t)
	courseID := createDraftCourse(t, env.router, env.teacher, "T87 архив")
	m1 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 1")
	l1 := t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 1")
	m2 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 2")
	l2 := t87AddLesson(t, env.router, env.teacher, courseID, m2, "Урок 2")
	t87Submit(t, env.router, env.teacher, courseID)
	t87PublishOK(t, env.router, env.admin, courseID)

	rec := doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/archive", nil, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}

	base := "/api/v1/teacher/courses/" + courseID
	// Все контентные операции admin по архиву — 409.
	rec = doRequest(t, env.router, http.MethodPatch, base+"/lessons/"+l1,
		map[string]any{"title": "нельзя"}, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin PATCH урока archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost, base+"/modules",
		map[string]any{"title": "нельзя"}, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin POST модуля archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost, base+"/lessons",
		map[string]any{"moduleId": m1, "title": "нельзя", "kind": "video_text", "bodyMd": "x"}, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin POST урока archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodDelete, base+"/lessons/"+l2, nil, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin DELETE урока archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodDelete, base+"/modules/"+m2, nil, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin DELETE модуля archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost, base+"/modules/reorder", map[string]any{
		"items": []any{
			map[string]any{"id": m1, "position": 2},
			map[string]any{"id": m2, "position": 1},
		},
	}, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin reorder модулей archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost, base+"/lessons/reorder", map[string]any{
		"items": []any{
			map[string]any{"id": l1, "position": 1},
			map[string]any{"id": l2, "position": 1},
		},
	}, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin reorder уроков archived: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost, base+"/lessons/"+l1+"/move",
		map[string]any{"moduleId": m2}, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin move урока archived: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Force publish архивного курса — 409.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/publish", nil, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("publish archived: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// После unarchive (archived → draft) правка снова доступна.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/unarchive", nil, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPatch, base+"/lessons/"+l1,
		map[string]any{"title": "После unarchive"}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PATCH после unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Сценарий 5: POST /admin/courses/{id}/publish ----

func TestAdminCoursePublish(t *testing.T) {
	env := t87NewEnv(t)

	// Курс на модерации; отклоняем с комментарием — draft + moderationComment.
	courseID := createDraftCourse(t, env.router, env.teacher, "T87 force publish")
	m1 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 1")
	t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 1")
	t87Submit(t, env.router, env.teacher, courseID)

	const comment = "Нужно доработать описание"
	rec := doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/moderate",
		map[string]any{"decision": "reject", "comment": comment}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate reject: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if c, _ := env.app.Store.GetCourseByID(courseID); c.Status != repo.CourseDraft || c.ModerationComment != comment {
		t.Fatalf("после reject ждали draft с комментарием: %+v", c)
	}

	// Force publish из draft: 200, published, комментарий очищен.
	course := t87PublishOK(t, env.router, env.admin, courseID)
	if course["id"] != courseID || course["status"] != "published" {
		t.Fatalf("force publish draft: %v", course)
	}
	if v, ok := course["moderationComment"]; ok && v != "" {
		t.Fatalf("moderationComment должен очиститься: %v", course)
	}
	stored, _ := env.app.Store.GetCourseByID(courseID)
	if stored.Status != repo.CoursePublished || stored.ModerationComment != "" {
		t.Fatalf("после force publish: %+v", stored)
	}

	// Идемпотентность: повторный publish — 200, статус published.
	again := t87PublishOK(t, env.router, env.admin, courseID)
	if again["status"] != "published" {
		t.Fatalf("повторный publish: %v", again)
	}

	// Force publish из review: второй курс с программой.
	reviewID := createDraftCourse(t, env.router, env.teacher, "T87 force publish review")
	rm := t87AddModule(t, env.router, env.teacher, reviewID, "Модуль 1")
	t87AddLesson(t, env.router, env.teacher, reviewID, rm, "Урок 1")
	t87Submit(t, env.router, env.teacher, reviewID)
	reviewCourse := t87PublishOK(t, env.router, env.admin, reviewID)
	if reviewCourse["status"] != "published" {
		t.Fatalf("force publish review: %v", reviewCourse)
	}

	// Пустой draft тоже публикуется: force publish не повторяет submit-guard.
	emptyID := createDraftCourse(t, env.router, env.teacher, "T87 пустой draft")
	if emptyCourse := t87PublishOK(t, env.router, env.admin, emptyID); emptyCourse["status"] != "published" {
		t.Fatalf("force publish пустого draft: %v", emptyCourse)
	}

	// Архивный курс — 409.
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/archive", nil, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive перед publish: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/publish", nil, env.admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("publish archived: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Нет курса — 404 (в том числе не-UUID).
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "fake-id"} {
		rec = doRequest(t, env.router, http.MethodPost,
			"/api/v1/admin/courses/"+id+"/publish", nil, env.admin)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Курс не найден") {
			t.Fatalf("publish %q: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
	}

	// Права: teacher — 403, аноним — 401.
	draftID := createDraftCourse(t, env.router, env.teacher, "T87 права publish")
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+draftID+"/publish", nil, env.teacher)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher publish: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+draftID+"/publish", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним publish: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Сценарий 6: moderationComment в GET /admin/courses/{id} ----

func TestAdminCourseDetailModerationComment(t *testing.T) {
	env := t87NewEnv(t)
	courseID := createDraftCourse(t, env.router, env.teacher, "T87 комментарий модерации")
	m1 := t87AddModule(t, env.router, env.teacher, courseID, "Модуль 1")
	t87AddLesson(t, env.router, env.teacher, courseID, m1, "Урок 1")

	// Без комментария — ключ присутствует и пустой.
	body := adminCourseDetail(t, env.router, env.admin, courseID)
	course, ok := body["course"].(map[string]any)
	if !ok {
		t.Fatalf("ответ без course: %v", body)
	}
	commentValue, present := course["moderationComment"]
	if !present || commentValue != "" {
		t.Fatalf("moderationComment без замечаний: present=%v value=%v", present, commentValue)
	}

	// После reject — текст замечания.
	t87Submit(t, env.router, env.teacher, courseID)
	const comment = "Поправьте программу курса"
	rec := doRequest(t, env.router, http.MethodPost,
		"/api/v1/admin/courses/"+courseID+"/moderate",
		map[string]any{"decision": "reject", "comment": comment}, env.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = adminCourseDetail(t, env.router, env.admin, courseID)
	course = body["course"].(map[string]any)
	if course["moderationComment"] != comment {
		t.Fatalf("moderationComment=%v, ждали %q", course["moderationComment"], comment)
	}

	// Force publish очищает комментарий — и в деталях снова пусто.
	t87PublishOK(t, env.router, env.admin, courseID)
	body = adminCourseDetail(t, env.router, env.admin, courseID)
	course = body["course"].(map[string]any)
	if course["status"] != "published" {
		t.Fatalf("статус после publish: %v", course["status"])
	}
	if commentValue, present := course["moderationComment"]; !present || commentValue != "" {
		t.Fatalf("moderationComment после publish: present=%v value=%v", present, commentValue)
	}
}
