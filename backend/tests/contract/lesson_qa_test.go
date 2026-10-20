package contract_test

// T71 — вопросы и ответы по урокам (волна P9).
// Контракт — Task/00-api-contract.md, раздел «T71 — Вопросы и ответы по
// урокам» (заморожен):
//   - публичный GET /courses/{id}/questions с фильтром lessonId и пагинацией;
//   - POST вопрос/ответ — записанные, владелец курса, admin;
//   - DELETE вопроса/ответа — автор, владелец, admin;
//   - вычисляемые lessonTitle/studentName/authorName/authorRole/isTeacher;
//   - неопубликованный курс для не-владельца/не-админа — 404.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
)

// qaLessonIDs — id уроков курса в порядке программы.
func qaLessonIDs(t *testing.T, app *handler.App, courseID string) []string {
	t.Helper()
	lessons := app.Store.ListLessonsByCourse(courseID)
	ids := make([]string, 0, len(lessons))
	for _, l := range lessons {
		ids = append(ids, l.ID)
	}
	return ids
}

// qaEnrollStudent регистрирует ученика и записывает его на бесплатный курс.
func qaEnrollStudent(t *testing.T, router http.Handler, courseID, email string) ([]*http.Cookie, string) {
	t.Helper()
	cookies, id := registerUser(t, router, email, "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll %s: status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	return cookies, id
}

// qaAddLesson добавляет video_text-урок в первый модуль курса.
func qaAddLesson(t *testing.T, app *handler.App, router http.Handler, teacherCookies []*http.Cookie, courseID, title string) string {
	t.Helper()
	modules := app.Store.ListModulesByCourse(courseID)
	if len(modules) == 0 {
		t.Fatalf("у курса %s нет модулей", courseID)
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": modules[0].ID, "title": title, "kind": "video_text", "bodyMd": "текст урока",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	id, _ := lesson["id"].(string)
	if id == "" {
		t.Fatalf("POST lesson без id: %s", rec.Body.String())
	}
	return id
}

// qaDraftCourseWithLesson — чужой учитель, его черновик, модуль и урок.
// Нужен для проверок «урок/вопрос другого курса» и видимости черновика.
func qaDraftCourseWithLesson(t *testing.T, app *handler.App, router http.Handler, tag string) (teacherCookies []*http.Cookie, courseID, lessonID string) {
	t.Helper()
	teacherCookies, courseID = seedTeacherCourse(t, app, router, tag)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Модуль " + tag,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module %s: status=%d body=%s", tag, rec.Code, rec.Body.String())
	}
	lessonID = qaAddLesson(t, app, router, teacherCookies, courseID, "Урок "+tag)
	return teacherCookies, courseID, lessonID
}

// qaPostQuestion создаёт вопрос и возвращает объект question из конверта.
func qaPostQuestion(t *testing.T, router http.Handler, courseID, lessonID, body string, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/questions", map[string]any{
		"lessonId": lessonID, "body": body,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create question: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	decodeBody(t, rec, &envelope)
	q, _ := envelope["question"].(map[string]any)
	if q == nil {
		t.Fatalf("в ответе нет question: %s", rec.Body.String())
	}
	if _, ok := q["answers"].([]any); !ok {
		t.Fatalf("answers не массив: %s", rec.Body.String())
	}
	if items, _ := q["answers"].([]any); len(items) != 0 {
		t.Fatalf("у нового вопроса непустой answers: %s", rec.Body.String())
	}
	return q
}

// qaListQuestions — GET ленты вопросов (ожидает 200) и декодирует конверт.
func qaListQuestions(t *testing.T, router http.Handler, path string, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, path, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// qaItems достаёт items конверта как []any (проверяя, что это массив).
func qaItems(t *testing.T, envelope map[string]any) []any {
	t.Helper()
	items, ok := envelope["items"].([]any)
	if !ok {
		t.Fatalf("items не массив: %v", envelope["items"])
	}
	return items
}

// qaQuestionByID ищет вопрос в ленте по id.
func qaQuestionByID(t *testing.T, envelope map[string]any, questionID string) map[string]any {
	t.Helper()
	for _, raw := range qaItems(t, envelope) {
		q, _ := raw.(map[string]any)
		if q["id"] == questionID {
			return q
		}
	}
	t.Fatalf("вопрос %s не найден в ленте: %v", questionID, envelope)
	return nil
}

// qaErrText проверяет код и точный текст ошибки контракта.
func qaErrText(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, wantText string) {
	t.Helper()
	if rec.Code != wantCode {
		t.Fatalf("status=%d, ждали %d: %s", rec.Code, wantCode, rec.Body.String())
	}
	var body map[string]string
	decodeBody(t, rec, &body)
	if body["error"] != wantText {
		t.Fatalf("error=%q, ждали %q: %s", body["error"], wantText, rec.Body.String())
	}
}

// TestLessonQuestionsPublicListAndPagination — публичная лента, фильтр по
// уроку, порядок (вопросы новые первыми), пагинация и дефолты page/limit.
func TestLessonQuestionsPublicListAndPagination(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	lessons := qaLessonIDs(t, app, courseID)
	if len(lessons) != 1 {
		t.Fatalf("уроков в seed-курсе: %d, ждали 1", len(lessons))
	}
	firstLesson := lessons[0]
	secondLesson := qaAddLesson(t, app, router, teacherCookies, courseID, "Второй урок")

	// Пустая лента: items — массив, не null; page/limit — дефолты.
	empty := qaListQuestions(t, router, "/api/v1/courses/"+courseID+"/questions", nil)
	if empty["total"] != float64(0) || empty["page"] != float64(1) || empty["limit"] != float64(20) {
		t.Fatalf("пустая лента: %s", empty)
	}
	if items := qaItems(t, empty); len(items) != 0 {
		t.Fatalf("items пустой ленты=%d, ждали 0", len(items))
	}

	s1Cookies, s1ID := qaEnrollStudent(t, router, courseID, "qa-s1@example.com")
	s2Cookies, _ := qaEnrollStudent(t, router, courseID, "qa-s2@example.com")
	// displayName читается из users.display_name — проверяем в studentName.
	if !app.Store.SetUserDisplayName(s1ID, "Анна Петрова") {
		t.Fatal("не удалось задать displayName ученику")
	}

	q1 := qaPostQuestion(t, router, courseID, firstLesson, "Почему переменная называется переменной?", s1Cookies)
	if q1["lessonTitle"] != "Переменные" {
		t.Fatalf("lessonTitle=%v, ждали «Переменные»", q1["lessonTitle"])
	}
	if q1["studentName"] != "Анна Петрова" || q1["studentId"] != s1ID {
		t.Fatalf("studentName/studentId=%v/%v: %s", q1["studentName"], q1["studentId"], q1["body"])
	}
	if q1["courseId"] != courseID || q1["lessonId"] != firstLesson {
		t.Fatalf("courseId/lessonId=%v/%v", q1["courseId"], q1["lessonId"])
	}
	q2 := qaPostQuestion(t, router, courseID, secondLesson, "А для чего нужен второй урок?", s2Cookies)
	q3 := qaPostQuestion(t, router, courseID, firstLesson, "Можно пример с областью видимости?", s1Cookies)

	all := qaListQuestions(t, router, "/api/v1/courses/"+courseID+"/questions", nil)
	if all["total"] != float64(3) {
		t.Fatalf("total=%v, ждали 3: %v", all["total"], all)
	}
	items := qaItems(t, all)
	if len(items) != 3 {
		t.Fatalf("items=%d, ждали 3", len(items))
	}
	if first, _ := items[0].(map[string]any); first["id"] != q3["id"] {
		t.Fatalf("первым должен быть новый вопрос %v, got %v", q3["id"], first["id"])
	}
	if last, _ := items[2].(map[string]any); last["id"] != q1["id"] {
		t.Fatalf("последним должен быть старый вопрос %v, got %v", q1["id"], last["id"])
	}

	// Фильтр по уроку: только вопросы этого урока, total — по фильтру.
	filtered := qaListQuestions(t, router,
		"/api/v1/courses/"+courseID+"/questions?lessonId="+firstLesson, nil)
	if filtered["total"] != float64(2) {
		t.Fatalf("total по уроку=%v, ждали 2", filtered["total"])
	}
	for _, raw := range qaItems(t, filtered) {
		q, _ := raw.(map[string]any)
		if q["lessonId"] != firstLesson {
			t.Fatalf("в фильтре чужой урок: %v", q["lessonId"])
		}
		if q["id"] == q2["id"] {
			t.Fatal("вопрос второго урока попал в фильтр")
		}
	}

	// Урок другого курса — 404 с контрактным текстом.
	_, _, foreignLesson := qaDraftCourseWithLesson(t, app, router, "qa-foreign-lesson")
	rec := doRequest(t, router, http.MethodGet,
		"/api/v1/courses/"+courseID+"/questions?lessonId="+foreignLesson, nil, nil)
	qaErrText(t, rec, http.StatusNotFound, "Урок не найден в этом курсе")

	// Пагинация: page=2&limit=1 — второй по порядку (q2), total до пагинации.
	page2 := qaListQuestions(t, router,
		"/api/v1/courses/"+courseID+"/questions?page=2&limit=1", nil)
	if page2["total"] != float64(3) || page2["page"] != float64(2) || page2["limit"] != float64(1) {
		t.Fatalf("конверт пагинации: %v", page2)
	}
	pageItems := qaItems(t, page2)
	if len(pageItems) != 1 || pageItems[0].(map[string]any)["id"] != q2["id"] {
		t.Fatalf("page=2&limit=1 вернул %v, ждали вопрос %v", pageItems, q2["id"])
	}

	// Нечисловые/вне диапазона page/limit — дефолты 1/20.
	for _, raw := range []string{"page=abc&limit=xyz", "page=0&limit=0", "page=-3&limit=101"} {
		envelope := qaListQuestions(t, router,
			"/api/v1/courses/"+courseID+"/questions?"+raw, nil)
		if envelope["page"] != float64(1) || envelope["limit"] != float64(20) {
			t.Fatalf("%s: page/limit=%v/%v, ждали 1/20", raw, envelope["page"], envelope["limit"])
		}
		if len(qaItems(t, envelope)) != 3 {
			t.Fatalf("%s: вернулось не все 3 вопроса", raw)
		}
	}

	// Страница за пределами — пустой массив, не null.
	tail := qaListQuestions(t, router,
		"/api/v1/courses/"+courseID+"/questions?page=99&limit=20", nil)
	if items := qaItems(t, tail); len(items) != 0 || tail["total"] != float64(3) {
		t.Fatalf("хвост пагинации: %v", tail)
	}
}

// TestLessonQuestionCreatePermissionsAndValidation — 401/403/201, владелец и
// admin, урок другого курса, валидация длин, неопубликованный курс.
func TestLessonQuestionCreatePermissionsAndValidation(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	lessonID := qaLessonIDs(t, app, courseID)[0]
	path := "/api/v1/courses/" + courseID + "/questions"

	// Гость — 401 (requireAuth).
	rec := doRequest(t, router, http.MethodPost, path, map[string]any{
		"lessonId": lessonID, "body": "Вопрос без входа в аккаунт",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("гость: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if list := qaListQuestions(t, router, path, nil); list["total"] != float64(0) {
		t.Fatalf("гость создал вопрос: %v", list)
	}

	// Вошедший без записи — 403, вопрос не создан.
	strangerCookies, _ := registerUser(t, router, "qa-stranger@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, path, map[string]any{
		"lessonId": lessonID, "body": "Я не записан, но очень хочу спросить",
	}, strangerCookies)
	qaErrText(t, rec, http.StatusForbidden, "Задавать вопросы могут только записанные на курс")
	if list := qaListQuestions(t, router, path, nil); list["total"] != float64(0) {
		t.Fatalf("вопрос без записи создан: %v", list)
	}

	// Владелец курса — можно.
	ownerQ := qaPostQuestion(t, router, courseID, lessonID,
		"Вопрос от преподавателя по своему уроку", teacherCookies)
	if _, ok := ownerQ["studentName"].(string); !ok {
		t.Fatalf("studentName владельца не строка: %v", ownerQ["studentName"])
	}

	// Admin — можно.
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	qaPostQuestion(t, router, courseID, lessonID,
		"Вопрос администратора по уроку курса", adminCookies)

	// Валидация длины вопроса: после trim 10..2000 символов.
	studentCookies, _ := qaEnrollStudent(t, router, courseID, "qa-validate@example.com")
	for _, bad := range []string{"", "   коротко   ", strings.Repeat("я", 9), strings.Repeat("я", 2001)} {
		rec = doRequest(t, router, http.MethodPost, path, map[string]any{
			"lessonId": lessonID, "body": bad,
		}, studentCookies)
		qaErrText(t, rec, http.StatusBadRequest, "Вопрос: от 10 до 2000 символов")
	}
	// Границы принимаются: ровно 10 и ровно 2000 символов после trim.
	for _, good := range []string{"  " + strings.Repeat("я", 10) + "  ", strings.Repeat("я", 2000)} {
		q := qaPostQuestion(t, router, courseID, lessonID, good, studentCookies)
		if q["body"] != strings.TrimSpace(good) {
			t.Fatalf("body не обрезан: %q", q["body"])
		}
	}

	// Урок другого курса — 404.
	_, _, foreignLesson := qaDraftCourseWithLesson(t, app, router, "qa-foreign-2")
	rec = doRequest(t, router, http.MethodPost, path, map[string]any{
		"lessonId": foreignLesson, "body": "Вопрос к чужому уроку этого курса",
	}, studentCookies)
	qaErrText(t, rec, http.StatusNotFound, "Урок не найден в этом курсе")

	// Черновик чужого учителя: гостю 404, владельцу — 200.
	draftTeacherCookies, draftCourseID, draftLessonID := qaDraftCourseWithLesson(t, app, router, "qa-draft")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+draftCourseID+"/questions", nil, nil)
	qaErrText(t, rec, http.StatusNotFound, "Курс не найден")
	ownerList := qaListQuestions(t, router, "/api/v1/courses/"+draftCourseID+"/questions", draftTeacherCookies)
	if ownerList["total"] != float64(0) {
		t.Fatalf("лента черновика у владельца: %v", ownerList)
	}
	qaPostQuestion(t, router, draftCourseID, draftLessonID,
		"Вопрос владельца в собственном черновике", draftTeacherCookies)
}

// TestLessonAnswersAndDeletionRights — ответы ученика/преподавателя/admin с
// isTeacher, права удаления (автор/владелец/admin), 403 чужому, 404 чужого
// вопроса/ответа, каскад ответов при удалении вопроса.
func TestLessonAnswersAndDeletionRights(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	lessonID := qaLessonIDs(t, app, courseID)[0]
	questionsPath := "/api/v1/courses/" + courseID + "/questions"

	authorCookies, authorID := qaEnrollStudent(t, router, courseID, "qa-author@example.com")
	otherCookies, otherID := qaEnrollStudent(t, router, courseID, "qa-other@example.com")
	if !app.Store.SetUserDisplayName(authorID, "Иван Вопросов") {
		t.Fatal("не удалось задать displayName автору")
	}
	if !app.Store.SetUserDisplayName(otherID, "Пётр Ответов") {
		t.Fatal("не удалось задать displayName отвечающему")
	}
	q := qaPostQuestion(t, router, courseID, lessonID,
		"Как работает замыкание в этом примере?", authorCookies)
	questionID := q["id"].(string)
	answersPath := questionsPath + "/" + questionID + "/answers"

	postAnswer := func(body string, cookies []*http.Cookie) map[string]any {
		t.Helper()
		rec := doRequest(t, router, http.MethodPost, answersPath, map[string]any{"body": body}, cookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create answer: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var envelope map[string]any
		decodeBody(t, rec, &envelope)
		a, _ := envelope["answer"].(map[string]any)
		if a == nil {
			t.Fatalf("в ответе нет answer: %s", rec.Body.String())
		}
		return a
	}

	// Ученик отвечает: роль student, бейджа преподавателя нет.
	studentAnswer := postAnswer("  Попробую объяснить своими словами.  ", otherCookies)
	if studentAnswer["authorRole"] != "student" || studentAnswer["isTeacher"] != false {
		t.Fatalf("ответ ученика: role=%v isTeacher=%v", studentAnswer["authorRole"], studentAnswer["isTeacher"])
	}
	if studentAnswer["body"] != "Попробую объяснить своими словами." {
		t.Fatalf("body ответа не обрезан: %q", studentAnswer["body"])
	}
	if studentAnswer["authorName"] != "Пётр Ответов" {
		t.Fatalf("authorName=%v, ждали displayName отвечающего", studentAnswer["authorName"])
	}

	// Преподаватель и admin: isTeacher=true.
	teacherAnswer := postAnswer("Замыкание захватывает переменную из области видимости.", teacherCookies)
	if teacherAnswer["authorRole"] != "teacher" || teacherAnswer["isTeacher"] != true {
		t.Fatalf("ответ преподавателя: role=%v isTeacher=%v", teacherAnswer["authorRole"], teacherAnswer["isTeacher"])
	}
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	adminAnswer := postAnswer("Дополню ответ преподавателя примером.", adminCookies)
	if adminAnswer["authorRole"] != "admin" || adminAnswer["isTeacher"] != true {
		t.Fatalf("ответ админа: role=%v isTeacher=%v", adminAnswer["authorRole"], adminAnswer["isTeacher"])
	}

	// Лента: ответы старыми первыми.
	envelope := qaListQuestions(t, router, questionsPath, nil)
	got := qaQuestionByID(t, envelope, questionID)
	answers, ok := got["answers"].([]any)
	if !ok || len(answers) != 3 {
		t.Fatalf("answers=%v (ok=%v), ждали 3", got["answers"], ok)
	}
	firstAnswer := answers[0].(map[string]any)
	if firstAnswer["id"] != studentAnswer["id"] {
		t.Fatalf("первым должен быть старый ответ %v, got %v", studentAnswer["id"], firstAnswer["id"])
	}
	if firstAnswer["authorName"] != "Пётр Ответов" {
		t.Fatalf("authorName=%v, ждали displayName отвечающего", firstAnswer["authorName"])
	}

	// Валидация длины ответа: после trim 3..2000.
	for _, bad := range []string{"", "  ok  ", "  я  ", strings.Repeat("я", 2001)} {
		rec := doRequest(t, router, http.MethodPost, answersPath, map[string]any{"body": bad}, otherCookies)
		qaErrText(t, rec, http.StatusBadRequest, "Ответ: от 3 до 2000 символов")
	}
	// Ровно 3 символа после trim — можно.
	postAnswer("  да!  ", otherCookies)

	// 403: ученик не может удалить чужой ответ (ответ преподавателя).
	rec := doRequest(t, router, http.MethodDelete,
		answersPath+"/"+teacherAnswer["id"].(string), nil, otherCookies)
	qaErrText(t, rec, http.StatusForbidden, "Нет прав на удаление")

	// Автор удаляет свой ответ — 204.
	rec = doRequest(t, router, http.MethodDelete,
		answersPath+"/"+studentAnswer["id"].(string), nil, otherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE своего ответа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Повторное удаление — 404.
	rec = doRequest(t, router, http.MethodDelete,
		answersPath+"/"+studentAnswer["id"].(string), nil, otherCookies)
	qaErrText(t, rec, http.StatusNotFound, "Ответ не найден")

	// Владелец курса удаляет чужой ответ, admin — оставшийся.
	rec = doRequest(t, router, http.MethodDelete,
		answersPath+"/"+teacherAnswer["id"].(string), nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE ответа владельцем: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete,
		answersPath+"/"+adminAnswer["id"].(string), nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE ответа админом: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 404: вопрос другого курса при создании ответа.
	foreignTeacherCookies, foreignCourseID, _ := qaDraftCourseWithLesson(t, app, router, "qa-answer-foreign")
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/courses/"+foreignCourseID+"/questions/"+questionID+"/answers",
		map[string]any{"body": "Ответ не на тот вопрос другого курса"}, foreignTeacherCookies)
	qaErrText(t, rec, http.StatusNotFound, "Вопрос не найден")

	// 403: посторонний ученик пытается удалить вопрос.
	rec = doRequest(t, router, http.MethodDelete, questionsPath+"/"+questionID, nil, otherCookies)
	qaErrText(t, rec, http.StatusForbidden, "Нет прав на удаление")

	// 404: удаление вопроса по чужому курсу в пути.
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/courses/"+foreignCourseID+"/questions/"+questionID, nil, foreignTeacherCookies)
	qaErrText(t, rec, http.StatusNotFound, "Вопрос не найден")

	// Автор удаляет вопрос — 204, ответы уходят каскадом.
	rec = doRequest(t, router, http.MethodDelete, questionsPath+"/"+questionID, nil, authorCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE вопроса автором: status=%d body=%s", rec.Code, rec.Body.String())
	}
	after := qaListQuestions(t, router, questionsPath, nil)
	if after["total"] != float64(0) || len(qaItems(t, after)) != 0 {
		t.Fatalf("после удаления вопроса лента не пуста: %v", after)
	}
	rec = doRequest(t, router, http.MethodDelete, questionsPath+"/"+questionID, nil, authorCookies)
	qaErrText(t, rec, http.StatusNotFound, "Вопрос не найден")

	// Владелец и admin удаляют любые вопросы — 204.
	qOwner := qaPostQuestion(t, router, courseID, lessonID, "Вопрос, который удалит владелец курса", authorCookies)
	rec = doRequest(t, router, http.MethodDelete, questionsPath+"/"+qOwner["id"].(string), nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE вопроса владельцем: status=%d body=%s", rec.Code, rec.Body.String())
	}
	qAdmin := qaPostQuestion(t, router, courseID, lessonID, "Вопрос, который удалит администратор", authorCookies)
	rec = doRequest(t, router, http.MethodDelete, questionsPath+"/"+qAdmin["id"].(string), nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE вопроса админом: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestLessonQuestionAndAnswerEditRights — правка текста вопроса/ответа:
// только автор (не владелец курса и не admin), trim, валидация длины,
// 404 для чужого курса/несуществующей записи, обновление в публичной ленте.
func TestLessonQuestionAndAnswerEditRights(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	lessonID := qaLessonIDs(t, app, courseID)[0]
	questionsPath := "/api/v1/courses/" + courseID + "/questions"

	authorCookies, _ := qaEnrollStudent(t, router, courseID, "qa-edit-author@example.com")
	otherCookies, _ := qaEnrollStudent(t, router, courseID, "qa-edit-other@example.com")
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	q := qaPostQuestion(t, router, courseID, lessonID,
		"Исходный текст вопроса для проверки правки", authorCookies)
	questionID := q["id"].(string)
	questionPath := questionsPath + "/" + questionID

	// Чужие (ученик, владелец курса, admin) не переписывают авторский текст.
	for name, cookies := range map[string][]*http.Cookie{
		"другой ученик": otherCookies,
		"владелец":      teacherCookies,
		"admin":         adminCookies,
	} {
		rec := doRequest(t, router, http.MethodPatch, questionPath,
			map[string]any{"body": "Чужой текст вопроса"}, cookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d, ждали 403: %s", name, rec.Code, rec.Body.String())
		}
	}

	// Автор правит вопрос: body обрезается, ответы/метаданные сохраняются.
	rec := doRequest(t, router, http.MethodPatch, questionPath,
		map[string]any{"body": "  Обновлённый текст вопроса про замыкания  "}, authorCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH вопроса: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	decodeBody(t, rec, &envelope)
	updated, _ := envelope["question"].(map[string]any)
	if updated["body"] != "Обновлённый текст вопроса про замыкания" {
		t.Fatalf("body после PATCH=%q", updated["body"])
	}
	if updated["id"] != questionID || updated["studentId"] != q["studentId"] {
		t.Fatalf("PATCH изменил метаданные вопроса: %v", updated)
	}

	// Валидация: после trim 10..2000 символов.
	for _, bad := range []string{"", "  коротко  ", strings.Repeat("я", 2001)} {
		rec := doRequest(t, router, http.MethodPatch, questionPath,
			map[string]any{"body": bad}, authorCookies)
		qaErrText(t, rec, http.StatusBadRequest, "Вопрос: от 10 до 2000 символов")
	}

	// 404: вопроса нет / он принадлежит другому курсу.
	missing := "00000000-0000-4000-8000-000000000000"
	rec = doRequest(t, router, http.MethodPatch, questionsPath+"/"+missing,
		map[string]any{"body": "Текст для несуществующего вопроса"}, authorCookies)
	qaErrText(t, rec, http.StatusNotFound, "Вопрос не найден")
	foreignTeacherCookies, foreignCourseID, foreignLessonID := qaDraftCourseWithLesson(t, app, router, "qa-edit-foreign")
	foreignQ := qaPostQuestion(t, router, foreignCourseID, foreignLessonID,
		"Вопрос другого курса для проверки", foreignTeacherCookies)
	rec = doRequest(t, router, http.MethodPatch,
		questionsPath+"/"+foreignQ["id"].(string),
		map[string]any{"body": "Текст в чужом курсе"}, authorCookies)
	qaErrText(t, rec, http.StatusNotFound, "Вопрос не найден")

	// Ответ: автор правит свой текст, остальные — 403.
	answersPath := questionPath + "/answers"
	rec = doRequest(t, router, http.MethodPost, answersPath,
		map[string]any{"body": "Исходный текст ответа"}, otherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create answer: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &envelope)
	answer, _ := envelope["answer"].(map[string]any)
	answerID := answer["id"].(string)
	answerPath := answersPath + "/" + answerID

	rec = doRequest(t, router, http.MethodPatch, answerPath,
		map[string]any{"body": "Чужой текст ответа"}, authorCookies)
	qaErrText(t, rec, http.StatusForbidden, "Нет прав на редактирование")
	rec = doRequest(t, router, http.MethodPatch, answerPath,
		map[string]any{"body": "  Обновлённый текст ответа  "}, otherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH ответа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &envelope)
	answer, _ = envelope["answer"].(map[string]any)
	if answer["body"] != "Обновлённый текст ответа" || answer["id"] != answerID {
		t.Fatalf("ответ после PATCH: %v", answer)
	}
	for _, bad := range []string{"", "  ok  ", strings.Repeat("я", 2001)} {
		rec := doRequest(t, router, http.MethodPatch, answerPath,
			map[string]any{"body": bad}, otherCookies)
		qaErrText(t, rec, http.StatusBadRequest, "Ответ: от 3 до 2000 символов")
	}
	rec = doRequest(t, router, http.MethodPatch, answersPath+"/"+missing,
		map[string]any{"body": "Текст для несуществующего ответа"}, otherCookies)
	qaErrText(t, rec, http.StatusNotFound, "Ответ не найден")

	// Публичная лента отдаёт обновлённые тексты.
	envelope = qaListQuestions(t, router, questionsPath, nil)
	listed := qaQuestionByID(t, envelope, questionID)
	if listed["body"] != "Обновлённый текст вопроса про замыкания" {
		t.Fatalf("лента: старый текст вопроса %q", listed["body"])
	}
	answers, _ := listed["answers"].([]any)
	if len(answers) != 1 || answers[0].(map[string]any)["body"] != "Обновлённый текст ответа" {
		t.Fatalf("лента: старый текст ответа %v", listed["answers"])
	}
}
