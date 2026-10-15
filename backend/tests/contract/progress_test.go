package contract_test

// Контрактные тесты прогресса (specs T034; перенос + расширение).
// Здесь живут тесты прохождения уроков: квиз, code-задание (runner),
// форма ответа /me/courses и /complete.

import (
	"net/http"
	"testing"
)

// Прохождение quiz-урока: неверные ответы не засчитываются, верные — дают прогресс.
// (перенесено из payments_test.go без изменения логики проверок)
func TestProgressQuizLesson(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный: запись сразу

	studentCookies, _ := registerUser(t, router, "quiz@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll free: status=%d body=%s", rec.Code, rec.Body.String())
	}

	lessons := app.Store.ListLessonsByCourse(courseID)
	if len(lessons) == 0 {
		t.Fatal("в курсе нет уроков")
	}
	lessonID := lessons[0].ID

	// Неверный ответ.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 1}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("quiz wrong: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["passed"] != false {
		t.Fatalf("неверный ответ засчитан: %s", rec.Body.String())
	}

	// Верный ответ.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("quiz right: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["passed"] != true || out["percent"] != float64(100) {
		t.Fatalf("верный ответ не засчитан: %s", rec.Body.String())
	}
}

// Гарды CompleteLesson: quiz без ответов — 400, урок чужого курса — 400,
// урок не существует — 404, нет активной записи — 403.
func TestCompleteLessonGuards(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 10000) // платный: без покупки
	lessons := app.Store.ListLessonsByCourse(courseID)
	if len(lessons) == 0 {
		t.Fatal("в курсе нет уроков")
	}
	lessonID := lessons[0].ID

	studentCookies, _ := registerUser(t, router, "guarded@example.com", "password123", "")

	// Нет активной записи — 403 (сначала проверяется принадлежность урока,
	// он принадлежит курсу, поэтому дошли до проверки записи).
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("complete без записи: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Покупка (mock + webhook) открывает доступ.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var enroll map[string]any
	decodeBody(t, rec, &enroll)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": enroll["providerPaymentId"], "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Quiz без ответов — 400.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete",
		map[string]any{}, studentCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("quiz без ответов: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Урок из другого курса — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Чужой курс", "description": "Второй курс", "priceCents": 0,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var course2 map[string]any
	decodeBody(t, rec, &course2)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+course2["id"].(string)+"/modules", map[string]any{
		"title": "Модуль",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mod2 map[string]any
	decodeBody(t, rec, &mod2)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+course2["id"].(string)+"/lessons", map[string]any{
		"moduleId": mod2["id"], "title": "Чужой урок", "kind": "video_text", "bodyMd": "текст",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var lesson2 map[string]any
	decodeBody(t, rec, &lesson2)
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lesson2["id"].(string)+"/complete",
		map[string]any{}, studentCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("урок чужого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий урок — 404.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/00000000-0000-0000-0000-000000000000/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("несуществующий урок: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Code-урок при недоступном runner — 502 (RUNNER_URL указывает на закрытый порт).
func TestCompleteCodeLessonRunnerDown(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	// Добавляем code-урок в опубликованный курс.
	teacherID := userIDByEmail(t, app.Store, "teacher@example.com")
	course, _ := app.Store.GetCourseByID(courseID)
	if course.TeacherID != teacherID {
		t.Fatal("seedPublishedCourse создал курс не за того учителя")
	}
	mods := app.Store.ListModulesByCourse(courseID)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mods[0].ID, "title": "Сумма", "kind": "code",
		"codeTask": map[string]any{
			"language": "python", "template": "print(int(input())+1)",
			"tests": []any{map[string]any{"name": "пример", "input": "1", "expected": "2"}},
		},
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var codeLesson map[string]any
	decodeBody(t, rec, &codeLesson)

	// Запись активна (курс бесплатный).
	studentCookies, _ := registerUser(t, router, "coder@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Пустой код — 400 (до обращения к runner).
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+codeLesson["id"].(string)+"/complete",
		map[string]any{"code": "  "}, studentCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой код: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Runner недоступен — 502 (контракт: 502 при сбое проверки).
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+codeLesson["id"].(string)+"/complete",
		map[string]any{"code": "print(1)", "language": "python"}, studentCookies)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("runner down: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Форма GET /me/courses: конверт {items, total}, у каждого элемента —
// course + percent + nextLesson-объект (не nextLessonId).
func TestMyCoursesEnvelope(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0)

	studentCookies, _ := registerUser(t, router, "envelope@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// До прохождения: percent=0, nextLesson — объект.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mine map[string]any
	decodeBody(t, rec, &mine)
	items, _ := mine["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("ожидали 1 курс: %s", rec.Body.String())
	}
	item := items[0].(map[string]any)
	c, ok := item["course"].(map[string]any)
	if !ok || c["slug"] == "" || c["title"] == "" {
		t.Fatalf("нет вложенного course: %s", rec.Body.String())
	}
	if _, hasFlat := item["slug"]; hasFlat {
		t.Fatalf("в элементе не должно быть плоского slug: %s", rec.Body.String())
	}
	next, ok := item["nextLesson"].(map[string]any)
	if !ok || next["lessonId"] == "" || next["title"] == "" || next["kind"] == "" {
		t.Fatalf("nextLesson не объект {lessonId,title,kind}: %s", rec.Body.String())
	}

	// Проходим единственный урок → percent=100, nextLesson=null.
	lessons := app.Store.ListLessonsByCourse(courseID)
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessons[0].ID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	decodeBody(t, rec, &mine)
	item = mine["items"].([]any)[0].(map[string]any)
	if p, _ := item["percent"].(float64); p != 100 {
		t.Fatalf("percent=%v, body=%s", item["percent"], rec.Body.String())
	}
	if item["nextLesson"] != nil {
		t.Fatalf("пройденный курс: nextLesson должен быть null: %s", rec.Body.String())
	}
}
