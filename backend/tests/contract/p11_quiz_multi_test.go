package contract_test

// P11-8 (FR-010): множественный выбор в квизе.
// Проверяем аддитивный контракт: старые вопросы с correct работают как
// раньше, у multi-вопроса есть multi + correctIndexes, ответ ученика —
// число (старый клиент) или массив индексов (новый клиент). Правильные
// индексы не утекают в публичный вид урока.

import (
	"net/http"
	"strings"
	"testing"
)

func TestP11MultiQuizLesson(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, singleCourseID, _ := seedPublishedCourse(t, app, router, 0)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Новый черновик под multi-квиз.
	var course map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Множественный выбор", "description": "FR-010", "priceCents": 0,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &course)
	courseID, _ := course["id"].(string)
	slug, _ := course["slug"].(string)

	var mod map[string]any
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Основы",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &mod)
	moduleID, _ := mod["id"].(string)

	invalidQuestions := []struct {
		name  string
		extra map[string]any
	}{
		{"one correct index", map[string]any{"multi": true, "correctIndexes": []int{1}}},
		{"duplicate index", map[string]any{"multi": true, "correctIndexes": []int{1, 1}}},
		{"out of range", map[string]any{"multi": true, "correctIndexes": []int{1, 9}}},
	}
	for _, tc := range invalidQuestions {
		question := map[string]any{
			"text":    "Выберите чётные числа",
			"options": []string{"1", "2", "3", "4"},
		}
		for k, v := range tc.extra {
			question[k] = v
		}
		rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
			"moduleId": moduleID, "title": "Плохой вопрос " + tc.name, "kind": "quiz",
			"quiz": map[string]any{"questions": []any{question}},
		}, teacherCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("multi validation %q: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Некорректный тест") {
			t.Fatalf("multi validation %q: unexpected body=%s", tc.name, rec.Body.String())
		}
	}

	// Валидный multi-вопрос: correct не передаём, backend выводит его из
	// первого correctIndexes для обратной совместимости.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": moduleID, "title": "Чётные числа", "kind": "quiz",
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "m1", "text": "Выберите чётные числа",
			"options":        []string{"1", "2", "3", "4"},
			"multi":          true,
			"correctIndexes": []int{1, 3},
		}}},
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create multi lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var createdLesson map[string]any
	decodeBody(t, rec, &createdLesson)
	lessonID, _ := createdLesson["id"].(string)
	if lessonID == "" {
		t.Fatal("create multi lesson: empty id")
	}

	stored, ok := app.Store.GetLesson(lessonID)
	if !ok {
		t.Fatal("multi lesson not found in store")
	}
	q := stored.Quiz.Questions[0]
	if !q.Multi || len(q.CorrectIndexes) != 2 || q.CorrectIndexes[0] != 1 || q.CorrectIndexes[1] != 3 {
		t.Fatalf("stored multi question = %+v", q)
	}
	if q.Correct != 1 {
		t.Fatalf("legacy correct = %d, want first correctIndexes[0] = 1", q.Correct)
	}

	// Публикуем курс.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/moderate", map[string]any{
		"decision": "approve",
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Публичный вид: multi виден, correct/correctIndexes — нет.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page: status=%d body=%s", rec.Code, rec.Body.String())
	}
	publicBody := rec.Body.String()
	if !strings.Contains(publicBody, `"multi":true`) {
		t.Fatalf("public quiz lost multi flag: %s", publicBody)
	}
	if strings.Contains(publicBody, `"correct"`) || strings.Contains(publicBody, `"correctIndexes"`) {
		t.Fatalf("public quiz leaks correct answers: %s", publicBody)
	}

	// Запись на бесплатный курс и попытки ученика.
	studentCookies, _ := registerUser(t, router, "multi@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll multi course: status=%d body=%s", rec.Code, rec.Body.String())
	}

	complete := func(answers map[string]any) map[string]any {
		t.Helper()
		rec := doRequest(t, router, http.MethodPost,
			"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete",
			map[string]any{"quizAnswers": answers}, studentCookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("complete multi: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		decodeBody(t, rec, &out)
		return out
	}

	// Неполный набор и лишний вариант не засчитываются.
	if out := complete(map[string]any{"m1": []int{1}}); out["passed"] != false || out["completed"] != false {
		t.Fatalf("partial multi answer must fail: %+v", out)
	}
	if out := complete(map[string]any{"m1": []int{0, 1, 3}}); out["passed"] != false || out["completed"] != false {
		t.Fatalf("extra multi answer must fail: %+v", out)
	}
	// Точный набор в обратном порядке засчитывается: порядок не важен.
	out := complete(map[string]any{"m1": []int{3, 1}})
	if out["passed"] != true || out["completed"] != true {
		t.Fatalf("right multi answer must pass: %+v", out)
	}
	if percent, _ := out["percent"].(float64); percent != 100 {
		t.Fatalf("multi course percent = %v, want 100", out["percent"])
	}

	// Старый одиночный квиз из seedPublishedCourse продолжает работать с
	// числовым ответом (обратная совместимость).
	singleLessons := app.Store.ListLessonsByCourse(singleCourseID)
	if len(singleLessons) != 1 {
		t.Fatalf("single course lessons = %d, want 1", len(singleLessons))
	}
	singleLessonID := singleLessons[0].ID
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": singleCourseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll single course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+singleCourseID+"/lessons/"+singleLessonID+"/complete",
		map[string]any{"quizAnswers": map[string]any{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete single quiz: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var singleOut map[string]any
	decodeBody(t, rec, &singleOut)
	if singleOut["passed"] != true {
		t.Fatalf("legacy single answer must still pass: %+v", singleOut)
	}
}
