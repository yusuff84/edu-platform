package contract_test

// Контрактные тесты T21 — жизненный цикл курса в конструкторе: duplicate,
// archive/unarchive, withdraw, перенос урока между модулями. Сквозь
// NewRouter, как остальные тесты пакета (стиль teacher_test.go и helper_test.go).
//
// Проверяются: коды и тела из таблицы контракта, права (чужой курс → 403),
// машина состояний курса (409), глубокое копирование программы при duplicate
// и «хвост» списка уроков при переносе.

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// lifecycleTeacher регистрирует пользователя и выдаёт ему роль teacher
// (в проде роль даёт только инвайт, в тестах — прямая запись в store).
func lifecycleTeacher(t *testing.T, app *handler.App, router http.Handler, email string) ([]*http.Cookie, string) {
	t.Helper()
	cookies, id := registerUser(t, router, email, "password123", "")
	if !app.Store.SetUserRole(id, repo.RoleTeacher) {
		t.Fatalf("не удалось выдать роль teacher %s", email)
	}
	return cookies, id
}

// lifecycleCourse создаёт черновик курса с двумя модулями: в модуле 1 —
// video-урок (bodyMd/videoUrl) и quiz-урок (вопрос с вариантами и ответом),
// модуль 2 — пустой. Возвращает id курса, модулей и уроков.
func lifecycleCourse(t *testing.T, router http.Handler, cookies []*http.Cookie, title string) (courseID, m1, m2, l1, l2 string) {
	t.Helper()
	var course map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": title, "description": "Описание курса", "priceCents": 1500,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &course)
	courseID, _ = course["id"].(string)

	createModule := func(name string) string {
		t.Helper()
		var m map[string]any
		rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules",
			map[string]any{"title": name}, cookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
		}
		decodeBody(t, rec, &m)
		return m["id"].(string)
	}
	m1 = createModule("Модуль 1")
	m2 = createModule("Модуль 2")

	var lesson map[string]any
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": m1, "title": "Видеоурок", "kind": "video_text",
		"bodyMd": "**Жирный** текст урока", "videoUrl": "https://video.example/1",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create video lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &lesson)
	l1, _ = lesson["id"].(string)

	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": m1, "title": "Квиз", "kind": "quiz",
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "q1", "text": "2+2?", "options": []string{"3", "4"}, "correct": 1,
			"explanation": "Потому что четыре",
		}}},
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create quiz lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &lesson)
	l2, _ = lesson["id"].(string)
	return courseID, m1, m2, l1, l2
}

// 1. duplicate: копия — черновик вызывающего, программа совпадает (модули,
// уроки, quiz глубоко), slug свободный <slug>-copy-2, исходный курс не
// изменён; записи и прогресс студентов не копируются.
func TestLifecycleDuplicate(t *testing.T) {
	app, router := newTestApp()
	cookies, teacherID := lifecycleTeacher(t, app, router, "dup-teacher@example.com")
	courseID, _, _, l1, _ := lifecycleCourse(t, router, cookies, "Курс с программой")

	// Студент с записью и прогрессом на исходном курсе: копия не наследует.
	studentCookies, studentID := registerUser(t, router, "dup-student@example.com", "password123", "")
	_ = studentCookies
	if _, created := app.Store.EnsureEnrollment(studentID, courseID); !created {
		t.Fatal("не удалось создать запись студента на исходный курс")
	}
	app.Store.CompleteLesson(studentID, l1)

	src, ok := app.Store.GetCourseByID(courseID)
	if !ok {
		t.Fatal("исходный курс не найден")
	}
	srcSlug := src.Slug

	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/duplicate", nil, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("duplicate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Course map[string]any `json:"course"`
	}
	decodeBody(t, rec, &out)
	if out.Course == nil {
		t.Fatalf("ответ не в конверте {course}: %s", rec.Body.String())
	}
	copyID, _ := out.Course["id"].(string)
	if copyID == "" || copyID == courseID {
		t.Fatalf("id копии неверный: %q", copyID)
	}
	if out.Course["status"] != "draft" {
		t.Fatalf("копия должна быть draft: %v", out.Course["status"])
	}
	if out.Course["title"] != "Курс с программой (копия)" {
		t.Fatalf("название копии: %v", out.Course["title"])
	}
	if out.Course["teacherId"] != teacherID {
		t.Fatalf("владелец копии: %v, ждали %s", out.Course["teacherId"], teacherID)
	}
	if out.Course["slug"] != srcSlug+"-copy-2" {
		t.Fatalf("slug копии: %v, ждали %s-copy-2", out.Course["slug"], srcSlug)
	}
	if out.Course["avgRating"] != float64(0) || out.Course["ratingsCount"] != float64(0) {
		t.Fatalf("рейтинг копии должен быть сброшен: %s", rec.Body.String())
	}

	// Программа совпадает по количеству и содержимому; id — новые.
	srcMods := app.Store.ListModulesByCourse(courseID)
	copyMods := app.Store.ListModulesByCourse(copyID)
	if len(srcMods) != 2 || len(copyMods) != 2 {
		t.Fatalf("модули: исход=%d копия=%d", len(srcMods), len(copyMods))
	}
	for i := range srcMods {
		if copyMods[i].ID == srcMods[i].ID {
			t.Fatal("модули копии должны получить новые id")
		}
		if copyMods[i].Title != srcMods[i].Title || copyMods[i].Position != srcMods[i].Position {
			t.Fatalf("модуль %d копии не совпал: %+v vs %+v", i, copyMods[i], srcMods[i])
		}
	}
	srcLessons := app.Store.ListLessonsByModule(srcMods[0].ID)
	copyLessons := app.Store.ListLessonsByModule(copyMods[0].ID)
	if len(srcLessons) != 2 || len(copyLessons) != 2 {
		t.Fatalf("уроки: исход=%d копия=%d", len(srcLessons), len(copyLessons))
	}
	for i := range srcLessons {
		a, b := srcLessons[i], copyLessons[i]
		if a.ID == b.ID {
			t.Fatal("уроки копии должны получить новые id")
		}
		if a.Title != b.Title || a.Kind != b.Kind || a.Position != b.Position ||
			a.BodyMD != b.BodyMD || a.VideoURL != b.VideoURL || a.LiveURL != b.LiveURL {
			t.Fatalf("урок %d копии не совпал:\nкопия=%+v\nисход=%+v", i, b, a)
		}
		// Quiz — только у квиз-урока; глубокая копия сравнивается по значению
		// (у video-урока quiz нет вовсе — nil у обоих).
		if a.Quiz != nil || b.Quiz != nil {
			if a.Quiz == nil || b.Quiz == nil || !reflect.DeepEqual(a.Quiz.Questions, b.Quiz.Questions) {
				t.Fatalf("quiz %d копии не совпал: копия=%+v исход=%+v", i, b.Quiz, a.Quiz)
			}
		}
	}
	// Глубина копии: правка вариантов в копии не мутирует исходный урок.
	copyQuiz := copyLessons[1]
	if copyQuiz.Kind != repo.LessonQuiz {
		t.Fatalf("второй урок модуля должен быть квизом: %+v", copyQuiz)
	}
	copyQuiz.Quiz.Questions[0].Options[0] = "изменено в копии"
	if srcLessons[1].Quiz.Questions[0].Options[0] != "3" {
		t.Fatal("quiz скопирован не глубоко: правка копии изменила исходный курс")
	}
	if srcLessons[1].Title != "Квиз" || srcLessons[1].BodyMD != "" {
		t.Fatalf("поля исходного урока изменились: %+v", srcLessons[1])
	}

	// Исходный курс не изменился.
	srcNow, _ := app.Store.GetCourseByID(courseID)
	if srcNow.Status != repo.CourseDraft || srcNow.Slug != srcSlug || srcNow.Title != "Курс с программой" {
		t.Fatalf("исходный курс изменился: %+v", srcNow)
	}

	// Записи и прогресс не копируются.
	if _, found := app.Store.GetEnrollment(studentID, copyID); found {
		t.Fatal("запись студента не должна копироваться")
	}
	for _, l := range app.Store.ListLessonsByCourse(copyID) {
		if app.Store.IsLessonCompleted(studentID, l.ID) {
			t.Fatalf("прогресс по уроку %s не должен копироваться", l.ID)
		}
	}

	// 404 — курса нет.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/00000000-0000-0000-0000-000000000000/duplicate", nil, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("нет курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// 2. duplicate чужого курса: другой учитель — 403, студент — 403, аноним —
// 401; админ копирует чужой курс — 201, и копия числится за админом.
func TestLifecycleDuplicateForeign(t *testing.T) {
	app, router := newTestApp()
	ownerCookies, _ := lifecycleTeacher(t, app, router, "dup-owner@example.com")
	otherCookies, _ := lifecycleTeacher(t, app, router, "dup-other@example.com")
	courseID, _, _, _, _ := lifecycleCourse(t, router, ownerCookies, "Чужой курс")
	path := "/api/v1/teacher/courses/" + courseID + "/duplicate"

	rec := doRequest(t, router, http.MethodPost, path, nil, otherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой учитель: status=%d body=%s", rec.Code, rec.Body.String())
	}
	studentCookies, _ := registerUser(t, router, "dup-foreign-student@example.com", "password123", "")
	if rec := doRequest(t, router, http.MethodPost, path, nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("студент: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost, path, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Админ: копия создаётся и принадлежит админу (T21: «даже если копируют
	// админ чужой курс»).
	adminCookies, adminID := registerUser(t, router, "dup-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "dup-admin@example.com")
	rec = doRequest(t, router, http.MethodPost, path, nil, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("админ: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Course map[string]any `json:"course"`
	}
	decodeBody(t, rec, &out)
	if out.Course["teacherId"] != adminID {
		t.Fatalf("копия админа должна принадлежать админу: teacherId=%v, ждали %s",
			out.Course["teacherId"], adminID)
	}
	if found := app.Store.ListCoursesByTeacher(adminID); len(found) != 1 {
		t.Fatalf("у админа должен появиться ровно 1 свой курс: %d", len(found))
	}
}

// 3–4. archive/unarchive: published → archived, повторный archive → 409;
// archived → draft, unarchive из draft → 409; черновик тоже архивируется;
// чужой курс → 403, несуществующий → 404.
func TestLifecycleArchiveUnarchive(t *testing.T) {
	app, router := newTestApp()
	cookies, publishedID, _ := seedPublishedCourse(t, app, router, 0)

	var out struct {
		Course map[string]any `json:"course"`
	}

	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+publishedID+"/archive", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out.Course["status"] != "archived" {
		t.Fatalf("статус после archive: %v", out.Course["status"])
	}

	// Повторный archive — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+publishedID+"/archive", nil, cookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("повторный archive: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// unarchive → draft.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+publishedID+"/unarchive", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out.Course["status"] != "draft" {
		t.Fatalf("статус после unarchive: %v", out.Course["status"])
	}

	// unarchive из draft — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+publishedID+"/unarchive", nil, cookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("unarchive из draft: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Черновик тоже можно убрать в архив.
	draftID, _, _, _, _ := lifecycleCourse(t, router, cookies, "Черновик в архив")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+draftID+"/archive", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive draft: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out.Course["status"] != "archived" {
		t.Fatalf("статус черновика после archive: %v", out.Course["status"])
	}

	// Чужой учитель — 403.
	otherCookies, _ := lifecycleTeacher(t, app, router, "arch-other@example.com")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+draftID+"/archive", nil, otherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой учитель: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий курс — 404.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/00000000-0000-0000-0000-000000000000/archive", nil, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("нет курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// 5. withdraw: review → draft с очисткой комментария модерации; из published —
// 409; повторный withdraw — 409.
func TestLifecycleWithdraw(t *testing.T) {
	app, router := newTestApp()
	cookies, publishedID, _ := seedPublishedCourse(t, app, router, 0)

	// Из published отозвать нельзя — 409.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+publishedID+"/withdraw", nil, cookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("withdraw из published: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Новый курс: черновик → submit → review → withdraw → draft.
	reviewID, _, _, _, _ := lifecycleCourse(t, router, cookies, "Курс на модерации")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+reviewID+"/submit", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+reviewID+"/withdraw", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("withdraw: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Course map[string]any `json:"course"`
	}
	decodeBody(t, rec, &out)
	if out.Course["status"] != "draft" {
		t.Fatalf("статус после withdraw: %v", out.Course["status"])
	}
	if v, ok := out.Course["moderationComment"]; ok && v != "" {
		t.Fatalf("комментарий модерации должен очищаться: %v", v)
	}

	// Повторный withdraw (уже draft) — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+reviewID+"/withdraw", nil, cookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("повторный withdraw: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// 6. move: урок переезжает в конец другого модуля (200, позиция max+1),
// повторный перенос в тот же модуль — идемпотентный 200; пустой moduleId —
// 400 «Не указан модуль»; чужой модуль — 400; чужой урок — 404; перенос в
// published — 409 с текстом про черновик; чужой учитель — 403.
func TestLifecycleMoveLesson(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := lifecycleTeacher(t, app, router, "move-teacher@example.com")
	courseID, m1, m2, l1, l2 := lifecycleCourse(t, router, cookies, "Курс с переносом")

	// Второй модуль не пустой: «конец» — не первая позиция.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": m2, "title": "Урок во втором модуле", "kind": "video_text", "bodyMd": "текст",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	movePath := "/api/v1/teacher/courses/" + courseID + "/lessons/"

	var out struct {
		Lesson map[string]any `json:"lesson"`
	}

	// Перенос l1 в m2 → 200, модуль сменился, позиция — в конце (=2).
	rec = doRequest(t, router, http.MethodPost, movePath+l1+"/move", map[string]any{"moduleId": m2}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("move: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out.Lesson == nil {
		t.Fatalf("ответ не в конверте {lesson}: %s", rec.Body.String())
	}
	if out.Lesson["moduleId"] != m2 || out.Lesson["position"] != float64(2) {
		t.Fatalf("урок не переехал в конец модуля: %s", rec.Body.String())
	}
	if lessons := app.Store.ListLessonsByModule(m2); len(lessons) != 2 || lessons[1].ID != l1 {
		t.Fatalf("порядок целевого модуля: %+v", lessons)
	}

	// Идемпотентность: снова в тот же модуль — 200, позиция не изменилась.
	rec = doRequest(t, router, http.MethodPost, movePath+l1+"/move", map[string]any{"moduleId": m2}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("повторный move: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out.Lesson["position"] != float64(2) {
		t.Fatalf("идемпотентный перенос сдвинул позицию: %s", rec.Body.String())
	}

	// Пустой moduleId — 400 «Не указан модуль».
	rec = doRequest(t, router, http.MethodPost, movePath+l2+"/move", map[string]any{"moduleId": "   "}, cookies)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Не указан модуль") {
		t.Fatalf("пустой moduleId: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Модуль чужого курса — 400.
	foreignCookies, _ := lifecycleTeacher(t, app, router, "move-foreign@example.com")
	_, fm1, _, _, _ := lifecycleCourse(t, router, foreignCookies, "Другой курс")
	rec = doRequest(t, router, http.MethodPost, movePath+l2+"/move", map[string]any{"moduleId": fm1}, cookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("чужой модуль: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий модуль — 400.
	rec = doRequest(t, router, http.MethodPost, movePath+l2+"/move",
		map[string]any{"moduleId": "00000000-0000-0000-0000-000000000000"}, cookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("нет модуля: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Урок чужого курса — 404.
	foreignLessons := app.Store.ListLessonsByModule(fm1)
	if len(foreignLessons) == 0 {
		t.Fatal("у чужого курса нет уроков")
	}
	rec = doRequest(t, router, http.MethodPost, movePath+foreignLessons[0].ID+"/move",
		map[string]any{"moduleId": m1}, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужой урок: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий урок — 404.
	rec = doRequest(t, router, http.MethodPost,
		movePath+"00000000-0000-0000-0000-000000000000/move", map[string]any{"moduleId": m1}, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("нет урока: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Чужой учитель — 403.
	rec = doRequest(t, router, http.MethodPost, movePath+l2+"/move", map[string]any{"moduleId": m2}, foreignCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой учитель: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Курс не черновик (published) — 409 с точным текстом контракта.
	if !app.Store.SetCourseStatus(courseID, repo.CoursePublished, "") {
		t.Fatal("не удалось сменить статус курса")
	}
	rec = doRequest(t, router, http.MethodPost, movePath+l2+"/move", map[string]any{"moduleId": m2}, cookies)
	if rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "Менять структуру можно только в черновике") {
		t.Fatalf("published: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
