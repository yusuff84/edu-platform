package contract_test

// Контрактные тесты аудита: добавление модулей и уроков заморожено вне
// черновика. POST /teacher/courses/{id}/modules и .../lessons обязан требовать
// draft так же, как PATCH/DELETE/reorder (T19): published/archived — 409,
// причём guard срабатывает до разбора тела (пустое/битое тело не меняет ответ).
// Правило действует и для админа: ownCourse даёт ему доступ к чужому курсу, но
// не снимает заморозку программы.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

const draftOnlyContentError = "Контент курса можно редактировать только в режиме черновика"

// withDraftCourse временно переводит курс в draft, выполняет fn и возвращает
// исходный статус. Нужен тестам, чей сценарий требует опубликованный курс с
// дополнительным контентом: по новому правилу контент добавляется только в
// черновике, поэтому фикстура сначала снимает заморозку, а затем возвращает
// курс в исходное состояние.
func withDraftCourse(t *testing.T, app *handler.App, courseID string, fn func()) {
	t.Helper()
	c, ok := app.Store.GetCourseByID(courseID)
	if !ok {
		t.Fatalf("курс %s не найден", courseID)
	}
	original := c.Status
	if original != repo.CourseDraft {
		if !app.Store.SetCourseStatus(courseID, repo.CourseDraft, "") {
			t.Fatalf("не удалось перевести курс %s в draft", courseID)
		}
	}
	fn()
	if original != repo.CourseDraft {
		if !app.Store.SetCourseStatus(courseID, original, "") {
			t.Fatalf("не удалось вернуть курсу %s статус %s", courseID, original)
		}
	}
}

// assertDraftOnlyConflict проверяет 409 и замороженный текст ошибки.
func assertDraftOnlyConflict(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), draftOnlyContentError) {
		t.Fatalf("body=%s, ждали текст %q", rec.Body.String(), draftOnlyContentError)
	}
}

// TestTeacherCreateContentDraftGuard: draft -> 201 для модуля и урока,
// published/archived -> 409 даже без тела; новые строки не создаются.
func TestTeacherCreateContentDraftGuard(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := lifecycleTeacher(t, app, router, "content-guard-teacher@example.com")
	courseID, moduleID, _, _, _ := lifecycleCourse(t, router, cookies, "Заморозка программы")

	modulesPath := "/api/v1/teacher/courses/" + courseID + "/modules"
	lessonsPath := "/api/v1/teacher/courses/" + courseID + "/lessons"
	lessonBody := map[string]any{
		"moduleId": moduleID, "title": "Новый урок", "kind": "video_text", "bodyMd": "текст",
	}

	// Черновик: добавление разрешено.
	rec := doRequest(t, router, http.MethodPost, modulesPath, map[string]any{"title": "Новый модуль"}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("draft module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, lessonsPath, lessonBody, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("draft lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}

	for _, status := range []repo.CourseStatus{repo.CoursePublished, repo.CourseArchived} {
		if !app.Store.SetCourseStatus(courseID, status, "") {
			t.Fatalf("не удалось перевести курс в %s", status)
		}
		modulesBefore := len(app.Store.ListModulesByCourse(courseID))
		lessonsBefore := len(app.Store.ListLessonsByCourse(courseID))

		// Без тела: guard обязан сработать раньше decodeJSON (иначе был бы 400).
		rec = doRequest(t, router, http.MethodPost, modulesPath, nil, cookies)
		assertDraftOnlyConflict(t, rec)
		rec = doRequest(t, router, http.MethodPost, lessonsPath, nil, cookies)
		assertDraftOnlyConflict(t, rec)

		// Тело неверной формы/с невалидными полями тоже не должно доходить
		// до валидации: состояние курса важнее.
		rec = doRequest(t, router, http.MethodPost, modulesPath, "not-an-object", cookies)
		assertDraftOnlyConflict(t, rec)
		rec = doRequest(t, router, http.MethodPost, lessonsPath, map[string]any{"title": ""}, cookies)
		assertDraftOnlyConflict(t, rec)

		if got := len(app.Store.ListModulesByCourse(courseID)); got != modulesBefore {
			t.Fatalf("%s: модулей стало %d, было %d", status, got, modulesBefore)
		}
		if got := len(app.Store.ListLessonsByCourse(courseID)); got != lessonsBefore {
			t.Fatalf("%s: уроков стало %d, было %d", status, got, lessonsBefore)
		}
	}
}

// TestAdminCreateContentDraftGuard: админ тоже не может менять программу
// published/archived курса (403 не подменяет 409), хотя доступ к чужому курсу
// у него есть.
func TestAdminCreateContentDraftGuard(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	modulesPath := "/api/v1/teacher/courses/" + courseID + "/modules"
	lessonsPath := "/api/v1/teacher/courses/" + courseID + "/lessons"

	for _, status := range []repo.CourseStatus{repo.CoursePublished, repo.CourseArchived} {
		if !app.Store.SetCourseStatus(courseID, status, "") {
			t.Fatalf("не удалось перевести курс в %s", status)
		}
		for _, path := range []string{modulesPath, lessonsPath} {
			rec := doRequest(t, router, http.MethodPost, path, nil, adminCookies)
			if rec.Code != http.StatusConflict {
				t.Fatalf("админ, %s, %s: status=%d body=%s, ждали 409",
					status, path, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), draftOnlyContentError) {
				t.Fatalf("админ, %s, %s: body=%s", status, path, rec.Body.String())
			}
		}
	}

	// В черновике админ по-прежнему может помогать с программой — guard не
	// запрещает админский доступ как таковой, только правку вне draft.
	app.Store.SetCourseStatus(courseID, repo.CourseDraft, "")
	rec := doRequest(t, router, http.MethodPost, modulesPath, map[string]any{"title": "Админский модуль"}, adminCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("админ, draft module: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
