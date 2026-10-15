package handler

// Тесты T19 — CRUD конструктора: правка урока, удаление модулей/уроков/курса,
// reorder одной транзакцией, счётчики в GET /teacher/courses, права и
// draft-guard'ы. Существующие тесты (teacher_test.go контрактов,
// teacher_edit_test.go T13) не изменяются.

import (
	"encoding/json"
	"net/http"
	"testing"

	"edu-platform/backend/internal/repo"
)

// crudFixture поднимает роутер и черновик-курс у указанного учителя.
func crudFixture(t *testing.T, email string) (*App, http.Handler, *repo.User, *http.Cookie, *repo.Course) {
	t.Helper()
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, email, repo.RoleTeacher)
	cookie := sessionCookieFor(t, app, u.ID)
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: u.ID, Slug: "crud-course-1", Title: "Курс", Description: "Описание",
	})
	return app, router, u, cookie, c
}

// TestTeacherListCoursesWithCounts — обогащённый список моих курсов:
// items[].modulesCount/lessonsCount вместо N+1 запросов с фронта (T19).
func TestTeacherListCoursesWithCounts(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-list@x.ru")
	m1 := app.Store.CreateModule(c.ID, "Модуль 1")
	m2 := app.Store.CreateModule(c.ID, "Модуль 2")
	app.Store.CreateLesson(&repo.Lesson{ModuleID: m1.ID, Title: "Урок 1", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	app.Store.CreateLesson(&repo.Lesson{ModuleID: m1.ID, Title: "Урок 2", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	app.Store.CreateLesson(&repo.Lesson{ModuleID: m2.ID, Title: "Урок 3", Kind: repo.LessonVideoText, BodyMD: "Текст"})

	rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses", "", withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("список: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []struct {
			ID           string `json:"id"`
			ModulesCount int    `json:"modulesCount"`
			LessonsCount int    `json:"lessonsCount"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("разбор ответа: %v; body=%s", err, rec.Body.String())
	}
	if out.Total != 1 || len(out.Items) != 1 {
		t.Fatalf("ожидали 1 курс: %s", rec.Body.String())
	}
	if out.Items[0].ID != c.ID || out.Items[0].ModulesCount != 2 || out.Items[0].LessonsCount != 3 {
		t.Fatalf("счётчики неверные: %+v", out.Items[0])
	}
	// items никогда не null — фронт ожидает массив.
	if got := rec.Body.String(); len(got) < 10 || !containsJSONKey(got, `"items":[`) {
		t.Fatalf("items должен быть массивом, не null: %s", got)
	}

	// Пустой список тоже с массивом, а не null.
	other := mustUser(t, app, "t-list-empty@x.ru", repo.RoleTeacher)
	oc := sessionCookieFor(t, app, other.ID)
	rec = doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses", "", withCookie(oc))
	if rec.Code != http.StatusOK || !containsJSONKey(rec.Body.String(), `"items":[`) {
		t.Fatalf("пустой список: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Студенту — 403.
	sc := sessionCookieFor(t, app, mustUser(t, app, "t-list-student@x.ru", repo.RoleStudent).ID)
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses", "", withCookie(sc)); rec.Code != http.StatusForbidden {
		t.Fatalf("студент: status=%d, ждали 403", rec.Code)
	}
}

// containsJSONKey — грубая проверка подстроки в теле ответа.
func containsJSONKey(body, substr string) bool {
	return len(body) >= len(substr) && (indexOf(body, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestTeacherUpdateLesson — PATCH урока: слияние полей, валидация по kind,
// 400/404/409 и DisallowUnknownFields.
func TestTeacherUpdateLesson(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-lesson@x.ru")
	m := app.Store.CreateModule(c.ID, "Модуль")
	l := app.Store.CreateLesson(&repo.Lesson{
		ModuleID: m.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "Старый текст",
	})
	path := "/api/v1/teacher/courses/" + c.ID + "/lessons/" + l.ID

	// Успех: title + bodyMd, позиция и kind не меняются.
	rec := doJSON(t, router, http.MethodPatch, path,
		`{"title":"Новый урок","bodyMd":"Новый текст"}`, withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var upd repo.Lesson
	if err := json.Unmarshal(rec.Body.Bytes(), &upd); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if upd.Title != "Новый урок" || upd.BodyMD != "Новый текст" {
		t.Fatalf("поля не обновились: %+v", upd)
	}
	stored, ok := app.Store.GetLesson(l.ID)
	if !ok || stored.Title != "Новый урок" || stored.Kind != repo.LessonVideoText || stored.Position != l.Position {
		t.Fatalf("хранилище не обновилось: %+v", stored)
	}

	// Частичный PATCH не трогает остальное: videoUrl остаётся, title не передан.
	app.Store.UpdateLesson(&repo.Lesson{ID: l.ID, Title: "Урок", BodyMD: "Текст", VideoURL: "https://v.example/1"})
	if rec := doJSON(t, router, http.MethodPatch, path, `{"videoUrl":""}`, withCookie(cookie)); rec.Code != http.StatusOK {
		t.Fatalf("очистка videoUrl: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if stored, _ := app.Store.GetLesson(l.ID); stored.Title != "Урок" || stored.VideoURL != "" {
		t.Fatalf("частичный PATCH перезаписал лишнее: %+v", stored)
	}

	// Пустой title — 400.
	if rec := doJSON(t, router, http.MethodPatch, path, `{"title":"  "}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой title: status=%d", rec.Code)
	}
	// Смена kind — 400.
	if rec := doJSON(t, router, http.MethodPatch, path, `{"kind":"quiz"}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("смена kind: status=%d", rec.Code)
	}
	// Мусорный startsAt — 400.
	if rec := doJSON(t, router, http.MethodPatch, path, `{"startsAt":"не дата"}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("startsAt: status=%d", rec.Code)
	}
	// video_text без текста и видео — 400 (валидация по kind).
	if rec := doJSON(t, router, http.MethodPatch, path, `{"bodyMd":"","videoUrl":""}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой video_text: status=%d", rec.Code)
	}
	// Неизвестное поле — 400 (DisallowUnknownFields).
	if rec := doJSON(t, router, http.MethodPatch, path, `{"position":5}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("неизвестное поле: status=%d", rec.Code)
	}
	// Несуществующий урок — 404.
	if rec := doJSON(t, router, http.MethodPatch,
		"/api/v1/teacher/courses/"+c.ID+"/lessons/00000000-0000-0000-0000-000000000000",
		`{"title":"x"}`, withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("нет урока: status=%d", rec.Code)
	}
	// Урок чужого курса — 404.
	foreign := app.Store.CreateCourse(&repo.Course{TeacherID: "другой", Slug: "foreign-l", Title: "x", Description: "y"})
	fm := app.Store.CreateModule(foreign.ID, "Чужой")
	fl := app.Store.CreateLesson(&repo.Lesson{ModuleID: fm.ID, Title: "Чужой урок", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	if rec := doJSON(t, router, http.MethodPatch,
		"/api/v1/teacher/courses/"+c.ID+"/lessons/"+fl.ID,
		`{"title":"x"}`, withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("чужой урок: status=%d", rec.Code)
	}

	// Курс на модерации — 409.
	if !app.Store.SetCourseStatus(c.ID, repo.CourseReview, "") {
		t.Fatal("не удалось сменить статус")
	}
	if rec := doJSON(t, router, http.MethodPatch, path, `{"title":"x"}`, withCookie(cookie)); rec.Code != http.StatusConflict {
		t.Fatalf("review: status=%d, ждали 409", rec.Code)
	}
}

// TestTeacherDeleteModule — удаление модуля: 204, 409 при непустых уроках,
// 404 для чужого модуля, 409 вне черновика, права админа.
func TestTeacherDeleteModule(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-delmod@x.ru")
	empty := app.Store.CreateModule(c.ID, "Пустой")
	full := app.Store.CreateModule(c.ID, "Полный")
	app.Store.CreateLesson(&repo.Lesson{ModuleID: full.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	path := "/api/v1/teacher/courses/" + c.ID + "/modules/"

	// Непустой модуль — 409, модуль остаётся.
	if rec := doJSON(t, router, http.MethodDelete, path+full.ID, "", withCookie(cookie)); rec.Code != http.StatusConflict {
		t.Fatalf("непустой модуль: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := app.Store.GetModule(full.ID); !ok {
		t.Fatal("непустой модуль не должен удаляться")
	}

	// Пустой модуль — 204.
	if rec := doJSON(t, router, http.MethodDelete, path+empty.ID, "", withCookie(cookie)); rec.Code != http.StatusNoContent {
		t.Fatalf("пустой модуль: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := app.Store.GetModule(empty.ID); ok {
		t.Fatal("модуль не удалился")
	}

	// Чужой модуль (не из этого курса) — 404.
	foreign := app.Store.CreateCourse(&repo.Course{TeacherID: "другой", Slug: "foreign-m", Title: "x", Description: "y"})
	fm := app.Store.CreateModule(foreign.ID, "Чужой")
	if rec := doJSON(t, router, http.MethodDelete, path+fm.ID, "", withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("чужой модуль: status=%d", rec.Code)
	}

	// Чужой учитель — 403.
	other := mustUser(t, app, "t-delmod-other@x.ru", repo.RoleTeacher)
	oc := sessionCookieFor(t, app, other.ID)
	empty2 := app.Store.CreateModule(c.ID, "Ещё пустой")
	if rec := doJSON(t, router, http.MethodDelete, path+empty2.ID, "", withCookie(oc)); rec.Code != http.StatusForbidden {
		t.Fatalf("чужой учитель: status=%d", rec.Code)
	}

	// Админ от имени владельца может — 204.
	ac := sessionCookieFor(t, app, mustUser(t, app, "t-delmod-admin@x.ru", repo.RoleAdmin).ID)
	if rec := doJSON(t, router, http.MethodDelete, path+empty2.ID, "", withCookie(ac)); rec.Code != http.StatusNoContent {
		t.Fatalf("админ: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Курс на модерации — 409.
	app.Store.SetCourseStatus(c.ID, repo.CourseReview, "")
	blocked := app.Store.CreateModule(c.ID, "Заблокированный")
	if rec := doJSON(t, router, http.MethodDelete, path+blocked.ID, "", withCookie(cookie)); rec.Code != http.StatusConflict {
		t.Fatalf("review: status=%d, ждали 409", rec.Code)
	}
}

// TestTeacherDeleteLesson — удаление урока: 204, прогресс чистится,
// 404 для чужого урока, 409 вне черновика.
func TestTeacherDeleteLesson(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-dellesson@x.ru")
	m := app.Store.CreateModule(c.ID, "Модуль")
	l := app.Store.CreateLesson(&repo.Lesson{
		ModuleID: m.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "Текст",
	})
	student := mustUser(t, app, "t-dellesson-student@x.ru", repo.RoleStudent)
	app.Store.CompleteLesson(student.ID, l.ID)
	path := "/api/v1/teacher/courses/" + c.ID + "/lessons/"

	if rec := doJSON(t, router, http.MethodDelete, path+l.ID, "", withCookie(cookie)); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := app.Store.GetLesson(l.ID); ok {
		t.Fatal("урок не удалился")
	}
	if app.Store.IsLessonCompleted(student.ID, l.ID) {
		t.Fatal("прогресс по удалённому уроку должен исчезнуть")
	}

	// Несуществующий урок — 404.
	if rec := doJSON(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+c.ID+"/lessons/00000000-0000-0000-0000-000000000000",
		"", withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("нет урока: status=%d", rec.Code)
	}

	// Курс опубликован — 409.
	app.Store.SetCourseStatus(c.ID, repo.CoursePublished, "")
	l2 := app.Store.CreateLesson(&repo.Lesson{ModuleID: m.ID, Title: "Второй", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	if rec := doJSON(t, router, http.MethodDelete, path+l2.ID, "", withCookie(cookie)); rec.Code != http.StatusConflict {
		t.Fatalf("published: status=%d, ждали 409", rec.Code)
	}
}

// TestTeacherReorderModules — полный список позиций одной транзакцией:
// успех, идемпотентность, валидация (диапазон, дубликаты, полнота, 404/409).
func TestTeacherReorderModules(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-reorder@x.ru")
	m1 := app.Store.CreateModule(c.ID, "Первый")
	m2 := app.Store.CreateModule(c.ID, "Второй")
	m3 := app.Store.CreateModule(c.ID, "Третий")
	path := "/api/v1/teacher/courses/" + c.ID + "/modules/reorder"

	// Обратный порядок: 204 и перестановка применена.
	body := `{"items":[` +
		`{"id":"` + m3.ID + `","position":1},` +
		`{"id":"` + m2.ID + `","position":2},` +
		`{"id":"` + m1.ID + `","position":3}]}`
	if rec := doJSON(t, router, http.MethodPost, path, body, withCookie(cookie)); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: status=%d body=%s", rec.Code, rec.Body.String())
	}
	mods := app.Store.ListModulesByCourse(c.ID)
	if len(mods) != 3 || mods[0].ID != m3.ID || mods[1].ID != m2.ID || mods[2].ID != m1.ID {
		t.Fatalf("порядок не применился: %+v", mods)
	}

	// Идемпотентность: тот же список снова — 204, порядок тот же.
	if rec := doJSON(t, router, http.MethodPost, path, body, withCookie(cookie)); rec.Code != http.StatusNoContent {
		t.Fatalf("повтор: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if mods := app.Store.ListModulesByCourse(c.ID); mods[0].ID != m3.ID {
		t.Fatalf("повтор изменил порядок: %+v", mods)
	}

	// Неполный список — 400.
	if rec := doJSON(t, router, http.MethodPost, path,
		`{"items":[{"id":"`+m1.ID+`","position":1}]}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("неполный список: status=%d", rec.Code)
	}
	// Пустой/null — 400.
	if rec := doJSON(t, router, http.MethodPost, path, `{"items":null}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("null items: status=%d", rec.Code)
	}
	// Позиция вне диапазона — 400.
	bad := `{"items":[` +
		`{"id":"` + m3.ID + `","position":0},` +
		`{"id":"` + m2.ID + `","position":2},` +
		`{"id":"` + m1.ID + `","position":3}]}`
	if rec := doJSON(t, router, http.MethodPost, path, bad, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("позиция 0: status=%d", rec.Code)
	}
	// Дубликат позиции — 400.
	dup := `{"items":[` +
		`{"id":"` + m3.ID + `","position":1},` +
		`{"id":"` + m2.ID + `","position":1},` +
		`{"id":"` + m1.ID + `","position":3}]}`
	if rec := doJSON(t, router, http.MethodPost, path, dup, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("дубль позиции: status=%d", rec.Code)
	}
	// Дубликат id — 400.
	dupID := `{"items":[` +
		`{"id":"` + m3.ID + `","position":1},` +
		`{"id":"` + m3.ID + `","position":2},` +
		`{"id":"` + m1.ID + `","position":3}]}`
	if rec := doJSON(t, router, http.MethodPost, path, dupID, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("дубль id: status=%d", rec.Code)
	}
	// Чужой id — 404.
	foreign := app.Store.CreateModule("00000000-0000-0000-0000-000000000000", "Чужой")
	_ = foreign
	fm := app.Store.CreateCourse(&repo.Course{TeacherID: "другой", Slug: "reorder-foreign", Title: "x", Description: "y"})
	fmod := app.Store.CreateModule(fm.ID, "Чужой модуль")
	unknown := `{"items":[` +
		`{"id":"` + fmod.ID + `","position":1},` +
		`{"id":"` + m2.ID + `","position":2},` +
		`{"id":"` + m1.ID + `","position":3}]}`
	if rec := doJSON(t, router, http.MethodPost, path, unknown, withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("чужой id: status=%d", rec.Code)
	}

	// Курс на модерации — 409.
	app.Store.SetCourseStatus(c.ID, repo.CourseReview, "")
	if rec := doJSON(t, router, http.MethodPost, path, body, withCookie(cookie)); rec.Code != http.StatusConflict {
		t.Fatalf("review: status=%d, ждали 409", rec.Code)
	}
}

// TestTeacherReorderLessons — порядок уроков задаётся внутри модуля
// (UNIQUE (module_id, position)); полный список курса — одной транзакцией.
func TestTeacherReorderLessons(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-reorderl@x.ru")
	m1 := app.Store.CreateModule(c.ID, "Модуль 1")
	m2 := app.Store.CreateModule(c.ID, "Модуль 2")
	a1 := app.Store.CreateLesson(&repo.Lesson{ModuleID: m1.ID, Title: "A1", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	a2 := app.Store.CreateLesson(&repo.Lesson{ModuleID: m1.ID, Title: "A2", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	b1 := app.Store.CreateLesson(&repo.Lesson{ModuleID: m2.ID, Title: "B1", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	path := "/api/v1/teacher/courses/" + c.ID + "/lessons/reorder"

	// Меняем порядок уроков первого модуля (полный список курса).
	body := `{"items":[` +
		`{"id":"` + a2.ID + `","position":1},` +
		`{"id":"` + a1.ID + `","position":2},` +
		`{"id":"` + b1.ID + `","position":1}]}`
	if rec := doJSON(t, router, http.MethodPost, path, body, withCookie(cookie)); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: status=%d body=%s", rec.Code, rec.Body.String())
	}
	l1 := app.Store.ListLessonsByModule(m1.ID)
	if len(l1) != 2 || l1[0].ID != a2.ID || l1[1].ID != a1.ID {
		t.Fatalf("порядок уроков не применился: %+v", l1)
	}
	// Уроки второго модуля не тронуты: позиция 1, модуль тот же.
	if l2 := app.Store.ListLessonsByModule(m2.ID); len(l2) != 1 || l2[0].ID != b1.ID || l2[0].Position != 1 {
		t.Fatalf("второй модуль пострадал: %+v", l2)
	}

	// Позиция вне диапазона модуля (в m1 два урока, позиция 3) — 400.
	bad := `{"items":[` +
		`{"id":"` + a2.ID + `","position":3},` +
		`{"id":"` + a1.ID + `","position":2},` +
		`{"id":"` + b1.ID + `","position":1}]}`
	if rec := doJSON(t, router, http.MethodPost, path, bad, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("позиция вне диапазона: status=%d", rec.Code)
	}
	// Неполный список — 400.
	if rec := doJSON(t, router, http.MethodPost, path,
		`{"items":[{"id":"`+a1.ID+`","position":1}]}`, withCookie(cookie)); rec.Code != http.StatusBadRequest {
		t.Fatalf("неполный список: status=%d", rec.Code)
	}
	// Урок чужого курса — 404.
	fm := app.Store.CreateCourse(&repo.Course{TeacherID: "другой", Slug: "reorder-foreign-l", Title: "x", Description: "y"})
	fmod := app.Store.CreateModule(fm.ID, "Чужой")
	fl := app.Store.CreateLesson(&repo.Lesson{ModuleID: fmod.ID, Title: "Чужой", Kind: repo.LessonVideoText, BodyMD: "Текст"})
	unknown := `{"items":[` +
		`{"id":"` + a2.ID + `","position":1},` +
		`{"id":"` + a1.ID + `","position":2},` +
		`{"id":"` + fl.ID + `","position":1}]}`
	if rec := doJSON(t, router, http.MethodPost, path, unknown, withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("чужой урок: status=%d", rec.Code)
	}

	// Курс на модерации — 409.
	app.Store.SetCourseStatus(c.ID, repo.CourseReview, "")
	if rec := doJSON(t, router, http.MethodPost, path, body, withCookie(cookie)); rec.Code != http.StatusConflict {
		t.Fatalf("review: status=%d, ждали 409", rec.Code)
	}
}

// TestTeacherDeleteCourse — курс удаляется только в черновике; каскад чистит
// программу; права: владелец или админ.
func TestTeacherDeleteCourse(t *testing.T) {
	app, router, _, cookie, c := crudFixture(t, "t-delcourse@x.ru")
	m := app.Store.CreateModule(c.ID, "Модуль")
	app.Store.CreateLesson(&repo.Lesson{ModuleID: m.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "Текст"})

	// Чужой учитель — 403.
	other := mustUser(t, app, "t-delcourse-other@x.ru", repo.RoleTeacher)
	oc := sessionCookieFor(t, app, other.ID)
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+c.ID, "", withCookie(oc)); rec.Code != http.StatusForbidden {
		t.Fatalf("чужой учитель: status=%d", rec.Code)
	}

	// Владелец (draft) — 204, курс и программа исчезли.
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+c.ID, "", withCookie(cookie)); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := app.Store.GetCourseByID(c.ID); ok {
		t.Fatal("курс не удалился")
	}
	if _, ok := app.Store.GetModule(m.ID); ok {
		t.Fatal("модуль должен уйти каскадом")
	}
	if rec := doJSON(t, router, http.MethodGet, "/api/v1/teacher/courses/"+c.ID, "", withCookie(cookie)); rec.Code != http.StatusNotFound {
		t.Fatalf("после удаления: status=%d", rec.Code)
	}

	// Опубликованный курс удалить нельзя — 409.
	pub := app.Store.CreateCourse(&repo.Course{
		TeacherID: "другой", Slug: "published-del", Title: "x", Description: "y",
	})
	app.Store.SetCourseStatus(pub.ID, repo.CoursePublished, "")
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+pub.ID, "", withCookie(cookie)); rec.Code != http.StatusForbidden {
		t.Fatalf("чужой опубликованный: status=%d, ждали 403", rec.Code)
	}
	ownerCookie := sessionCookieFor(t, app, mustUser(t, app, "t-delcourse-pub@x.ru", repo.RoleTeacher).ID)
	_ = ownerCookie
	pub2 := app.Store.CreateCourse(&repo.Course{
		TeacherID: "другой", Slug: "published-del-2", Title: "x", Description: "y",
	})
	// Помечаем published, но удаляем от имени админа: 409 — только draft.
	app.Store.SetCourseStatus(pub2.ID, repo.CoursePublished, "")
	ac := sessionCookieFor(t, app, mustUser(t, app, "t-delcourse-admin@x.ru", repo.RoleAdmin).ID)
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+pub2.ID, "", withCookie(ac)); rec.Code != http.StatusConflict {
		t.Fatalf("published: status=%d, ждали 409", rec.Code)
	}

	// Админ может удалить чужой черновик — 204.
	draft := app.Store.CreateCourse(&repo.Course{
		TeacherID: "другой", Slug: "draft-for-admin", Title: "x", Description: "y",
	})
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+draft.ID, "", withCookie(ac)); rec.Code != http.StatusNoContent {
		t.Fatalf("админ удаляет чужой черновик: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestTeacherCRUDAuthGates — новые маршруты стоят под requireAuth/requireRole:
// аноним 401, студент 403 (в том числе на перезаписанном GET /teacher/courses).
func TestTeacherCRUDAuthGates(t *testing.T) {
	app, router, _, _, c := crudFixture(t, "t-gates@x.ru")
	m := app.Store.CreateModule(c.ID, "Модуль")
	l := app.Store.CreateLesson(&repo.Lesson{ModuleID: m.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "Текст"})

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/teacher/courses"},
		{http.MethodPatch, "/api/v1/teacher/courses/" + c.ID + "/lessons/" + l.ID},
		{http.MethodDelete, "/api/v1/teacher/courses/" + c.ID + "/modules/" + m.ID},
		{http.MethodDelete, "/api/v1/teacher/courses/" + c.ID + "/lessons/" + l.ID},
		{http.MethodPost, "/api/v1/teacher/courses/" + c.ID + "/modules/reorder"},
		{http.MethodPost, "/api/v1/teacher/courses/" + c.ID + "/lessons/reorder"},
		{http.MethodDelete, "/api/v1/teacher/courses/" + c.ID},
	}
	sc := sessionCookieFor(t, app, mustUser(t, app, "t-gates-student@x.ru", repo.RoleStudent).ID)
	for _, rt := range routes {
		if rec := doJSON(t, router, rt.method, rt.path, `{"title":"x","items":[]}`, nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s аноним: status=%d, ждали 401", rt.method, rt.path, rec.Code)
		}
		if rec := doJSON(t, router, rt.method, rt.path, `{"title":"x","items":[]}`, withCookie(sc)); rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s студент: status=%d, ждали 403", rt.method, rt.path, rec.Code)
		}
	}
}
