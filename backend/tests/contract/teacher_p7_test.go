package contract_test

// Контрактные тесты T53 (волна P7) — жизненный цикл учителя и жёсткая
// валидация: POST /teacher/courses/{id}/unpublish (published → draft, права
// как у archive/withdraw T21), startsAt у live-урока в двух форматах
// (RFC3339 или datetime-local, строка без зоны трактуется как UTC) и
// валидация quiz на бэкенде (POST и PATCH, ошибки «Некорректный тест: …»).
// Сквозь NewRouter, как остальные тесты пакета (стиль teacher_lifecycle_test.go).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// p7Publish поднимает статус существующего курса до published напрямую в
// store (машина модерации уже покрыта ранее: submit/approve — см.
// seedPublishedCourse; здесь проверяется только точка unpublish).
func p7Publish(t *testing.T, app *handler.App, courseID string) {
	t.Helper()
	if !app.Store.SetCourseStatus(courseID, repo.CoursePublished, "") {
		t.Fatal("не удалось перевести курс в published")
	}
}

// TestTeacherP7Unpublish — счастливый путь T53: у published курса снимают
// публикацию (published → draft, ответ в конверте {course}), тело запроса
// игнорируется, повторный unpublish — 409.
func TestTeacherP7Unpublish(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	var out struct {
		Course map[string]any `json:"course"`
	}

	// 1. Тело запроса игнорируется: с непустым телом — тоже 200.
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/unpublish",
		map[string]any{"reason": "просто хотим снять"}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("unpublish с телом: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out.Course == nil {
		t.Fatalf("ответ не в конверте {course}: %s", rec.Body.String())
	}
	if out.Course["id"] != courseID {
		t.Fatalf("конверт {course}: id=%v, ждали %s", out.Course["id"], courseID)
	}
	if out.Course["status"] != "draft" {
		t.Fatalf("статус после unpublish: %v, ждали draft", out.Course["status"])
	}

	// 2. Повторный unpublish (уже draft) — 409 с текстом контракта.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/unpublish", nil, teacherCookies)
	if rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "Снять с публикации можно только опубликованный курс") {
		t.Fatalf("повторный unpublish: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 3. После снятия публикации курс жив (draft), а не удалён/скрыт.
	c, ok := app.Store.GetCourseByID(courseID)
	if !ok || c.Status != repo.CourseDraft {
		t.Fatal("курс после unpublish должен остаться draft")
	}
}

// TestTeacherP7UnpublishGuards — охрана статуса и прав: draft/review/archived
// → 409; нет курса → 404; чужой учитель/студент → 403; аноним → 401;
// админ вправе снять с публикации чужой курс (200).
func TestTeacherP7UnpublishGuards(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := lifecycleTeacher(t, app, router, "p7-unpub-teacher@example.com")
	foreign, _ := lifecycleTeacher(t, app, router, "p7-unpub-other@example.com")
	student, _ := registerUser(t, router, "p7-unpub-student@example.com", "password123", "")

	// draft → 409.
	draftID, _, _, _, _ := lifecycleCourse(t, router, cookies, "P7 черновик")
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+draftID+"/unpublish", nil, cookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("draft unpublish: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// review → 409 (курс с программой отправлен на модерацию).
	reviewID, _, _, _, _ := lifecycleCourse(t, router, cookies, "P7 на модерации")
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+reviewID+"/submit", map[string]any{}, cookies); rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+reviewID+"/unpublish", nil, cookies); rec.Code != http.StatusConflict {
		t.Fatalf("review unpublish: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// archived → 409 (отзываем с модерации и убираем в архив).
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+reviewID+"/withdraw", nil, cookies); rec.Code != http.StatusOK {
		t.Fatalf("withdraw: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+reviewID+"/archive", nil, cookies); rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+reviewID+"/unpublish", nil, cookies); rec.Code != http.StatusConflict {
		t.Fatalf("archived unpublish: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий курс → 404 «Курс не найден».
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/00000000-0000-0000-0000-000000000000/unpublish", nil, cookies)
	if rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), "Курс не найден") {
		t.Fatalf("нет курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Чужой учитель → 403, студент → 403, аноним → 401.
	p7Publish(t, app, draftID)
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+draftID+"/unpublish", nil, foreign)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой учитель: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+draftID+"/unpublish", nil, student); rec.Code != http.StatusForbidden {
		t.Fatalf("студент: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+draftID+"/unpublish", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Админ (права как у archive/withdraw) снимает с публикации — 200 draft.
	admin, _ := registerUser(t, router, "p7-unpub-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "p7-unpub-admin@example.com")
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+draftID+"/unpublish", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("админ unpublish: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Course map[string]any `json:"course"`
	}
	decodeBody(t, rec, &out)
	if out.Course["status"] != "draft" {
		t.Fatalf("после unpublish админом: %s", rec.Body.String())
	}
}

// TestTeacherP7LessonStartsAt — startsAt у live-урока: POST и PATCH принимают
// RFC3339 и datetime-local (строка без зоны трактуется как UTC); мусор → 400;
// пустая строка очищает дату (поведение T19 не меняется).
func TestTeacherP7LessonStartsAt(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := lifecycleTeacher(t, app, router, "p7-starts-teacher@example.com")
	courseID, m1, _, _, _ := lifecycleCourse(t, router, cookies, "P7 планирование")

	// POST live-урока с datetime-local: строка без зоны → UTC.
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
			"moduleId": m1, "title": "Вебинар P7", "kind": "live",
			"liveUrl": "https://live.example/p7", "startsAt": "2026-11-01T09:30",
		}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST live datetime-local: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	lessonID, _ := lesson["id"].(string)
	if lessonID == "" {
		t.Fatalf("POST live: в ответе нет id: %s", rec.Body.String())
	}
	if s, _ := lesson["startsAt"].(string); s != "2026-11-01T09:30:00Z" {
		t.Fatalf("datetime-local на POST должен стать UTC: got %v", lesson["startsAt"])
	}
	stored, _ := app.Store.GetLesson(lessonID)
	if stored.StartsAt == nil || stored.StartsAt.Location() != time.UTC ||
		stored.StartsAt.Format(time.RFC3339) != "2026-11-01T09:30:00Z" {
		t.Fatalf("startsAt сохранился не в UTC: %+v", stored.StartsAt)
	}

	path := "/api/v1/teacher/courses/" + courseID + "/lessons/" + lessonID

	// PATCH datetime-local → 200, трактовка как UTC.
	rec = doRequest(t, router, http.MethodPatch, path, map[string]any{
		"startsAt": "2026-11-02T10:30",
	}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH datetime-local: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &lesson)
	if s, _ := lesson["startsAt"].(string); s != "2026-11-02T10:30:00Z" {
		t.Fatalf("PATCH datetime-local: ждали 2026-11-02T10:30:00Z, got %v", lesson["startsAt"])
	}

	// PATCH RFC3339 со смещением: смещение сохраняется, как и в POST.
	rec = doRequest(t, router, http.MethodPatch, path, map[string]any{
		"startsAt": "2030-03-01T12:00:00+03:00",
	}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH RFC3339: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &lesson)
	if s, _ := lesson["startsAt"].(string); s != "2030-03-01T12:00:00+03:00" {
		t.Fatalf("PATCH RFC3339 искажает смещение: got %v", lesson["startsAt"])
	}

	// Мусорный startsAt → 400 «Некорректный startsAt».
	rec = doRequest(t, router, http.MethodPatch, path, map[string]any{
		"startsAt": "завтра-вечером",
	}, cookies)
	if rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "Некорректный startsAt") {
		t.Fatalf("мусорный startsAt: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Пустая строка — явная очистка (T19).
	rec = doRequest(t, router, http.MethodPatch, path, map[string]any{"startsAt": ""}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("очистка startsAt: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if stored, _ := app.Store.GetLesson(lessonID); stored.StartsAt != nil {
		t.Fatalf("startsAt должен был очиститься: %+v", stored.StartsAt)
	}
}

// TestTeacherP7QuizValidation — валидация quiz на POST: пустой текст вопроса,
// один вариант, пустой вариант, правильный ответ вне границ (и как correct,
// и как correctIndex из контракта) — 400 «Некорректный тест: …»; корректный
// тест сохраняется (201) с описанием correct в ответе.
func TestTeacherP7QuizValidation(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := lifecycleTeacher(t, app, router, "p7-quiz-teacher@example.com")
	courseID, m1, _, _, _ := lifecycleCourse(t, router, cookies, "P7 тесты")

	// postQuiz шлёт один вопрос на создание quiz-урока.
	postQuiz := func(q map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		return doRequest(t, router, http.MethodPost,
			"/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
				"moduleId": m1, "title": "Тест P7", "kind": "quiz",
				"quiz": map[string]any{"questions": []any{q}},
			}, cookies)
	}
	quizBad := func(q map[string]any, name string) {
		t.Helper()
		if rec := postQuiz(q); rec.Code != http.StatusBadRequest ||
			!strings.Contains(rec.Body.String(), "Некорректный тест") {
			t.Fatalf("%s: status=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}

	// Пустой текст вопроса.
	quizBad(map[string]any{"text": "   ", "options": []string{"да", "нет"}, "correct": 1},
		"пустой текст вопроса")

	// Всего один вариант.
	quizBad(map[string]any{"text": "1+1?", "options": []string{"2"}, "correct": 0},
		"один вариант")

	// Пустой (после TrimSpace) вариант.
	quizBad(map[string]any{"text": "1+1?", "options": []string{"2", "  "}, "correct": 0},
		"пустой вариант")

	// correct вне границ.
	quizBad(map[string]any{"text": "1+1?", "options": []string{"2", "3"}, "correct": 5},
		"correct вне границ")

	// correctIndex (имя из контракта P7) вне границ — те же два варианта.
	quizBad(map[string]any{"text": "1+1?", "options": []string{"2", "3"}, "correctIndex": 5},
		"correctIndex вне границ")

	// Тест без вопросов.
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
			"moduleId": m1, "title": "Пустой тест", "kind": "quiz",
			"quiz": map[string]any{"questions": []any{}},
		}, cookies)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Некорректный тест") {
		t.Fatalf("quiz без вопросов: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Корректный тест сохраняется: 201, correct в ответе, id вопроса сгенерирован.
	rec = postQuiz(map[string]any{"text": "2*3?", "options": []string{"5", "6"}, "correct": 1})
	if rec.Code != http.StatusCreated {
		t.Fatalf("корректный quiz: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved struct {
		ID   string         `json:"id"`
		Quiz *repo.QuizData `json:"quiz"`
	}
	decodeBody(t, rec, &saved)
	if saved.ID == "" || saved.Quiz == nil || len(saved.Quiz.Questions) != 1 ||
		saved.Quiz.Questions[0].Correct != 1 || strings.TrimSpace(saved.Quiz.Questions[0].ID) == "" {
		t.Fatalf("quiz не сохранился корректно: %s", rec.Body.String())
	}

	// Форма контракта: correctIndex у валидного теста тоже принимается.
	if rec := postQuiz(map[string]any{"text": "1+2?", "options": []string{"3", "4"}, "correctIndex": 0}); rec.Code != http.StatusCreated {
		t.Fatalf("quiz с correctIndex: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestTeacherP7QuizPatchValidation — валидация quiz на PATCH урока: битый
// тест → 400 «Некорректный тест: …»; валидный (correctIndex из контракта) →
// 200, новая версия теста лежит в хранилище.
func TestTeacherP7QuizPatchValidation(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := lifecycleTeacher(t, app, router, "p7-quiz-patch@example.com")
	courseID, _, _, _, l2 := lifecycleCourse(t, router, cookies, "P7 правка теста")
	path := "/api/v1/teacher/courses/" + courseID + "/lessons/" + l2

	patchQuiz := func(q map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		return doRequest(t, router, http.MethodPatch, path, map[string]any{
			"quiz": map[string]any{"questions": []any{q}},
		}, cookies)
	}

	// Всего один вариант → 400 с описанием «Некорректный тест».
	rec := patchQuiz(map[string]any{"text": "2+2?", "options": []string{"4"}, "correct": 0})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Некорректный тест") {
		t.Fatalf("PATCH битого quiz: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// correctIndex вне границ → 400.
	rec = patchQuiz(map[string]any{"text": "2+2?", "options": []string{"4", "5"}, "correctIndex": 9})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH correctIndex вне границ: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Валидный PATCH — 200, обновлённый тест в хранилище.
	rec = patchQuiz(map[string]any{"text": "2+2?", "options": []string{"не четыре", "четыре"}, "correctIndex": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH валидного quiz: status=%d body=%s", rec.Code, rec.Body.String())
	}
	stored, _ := app.Store.GetLesson(l2)
	if stored.Quiz == nil || len(stored.Quiz.Questions) != 1 ||
		stored.Quiz.Questions[0].Correct != 1 || stored.Quiz.Questions[0].Text != "2+2?" {
		t.Fatalf("quiz не обновился / потерялся: %+v", stored.Quiz)
	}
}
