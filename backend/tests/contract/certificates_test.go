package contract_test

// T67 — контрактные тесты сертификатов о завершении курса:
// автоматическая выдача после последнего урока, идемпотентность, список
// /me/certificates, аддитивное поле certificateNumber в /me/courses и
// публичная проверка GET /certificates/{number} без авторизации.

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// certificateNumberRe — формат номера из контракта T67.
var certificateNumberRe = regexp.MustCompile(`^EDU-\d{4}-[0-9A-F]{8}$`)

// myCertificateItems — GET /me/certificates; проверяет форму конверта.
func myCertificateItems(t *testing.T, router http.Handler, cookies []*http.Cookie) []map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/certificates", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/certificates: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	raw, ok := out["items"].([]any)
	if !ok {
		t.Fatalf("items должен быть массивом (не null): %s", rec.Body.String())
	}
	items := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		items = append(items, it.(map[string]any))
	}
	return items
}

// certificateNumberInMyCourses — certificateNumber курса из GET /me/courses.
func certificateNumberInMyCourses(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	for _, raw := range out["items"].([]any) {
		item := raw.(map[string]any)
		course, _ := item["course"].(map[string]any)
		if course["id"] != courseID {
			continue
		}
		number, ok := item["certificateNumber"].(string)
		if !ok {
			t.Fatalf("в элементе /me/courses нет строкового certificateNumber: %s", rec.Body.String())
		}
		return number
	}
	t.Fatalf("курс %s не найден в /me/courses: %s", courseID, rec.Body.String())
	return ""
}

// Тест 1: сертификат выдаётся после завершения единственного урока, виден в
// /me/certificates и /me/courses, проверяется публично без авторизации;
// неизвестный номер — 404 с контрактным текстом.
func TestCertificateIssuedAfterLastLesson(t *testing.T) {
	app, router := newTestApp()
	_, courseID, courseSlug := seedPublishedCourse(t, app, router, 0)

	studentCookies, _ := registerUser(t, router, "cert@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", map[string]any{
		"displayName": "Иван Петров",
	}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: status=%d body=%s", rec.Code, rec.Body.String())
	}
	enrollFree(t, router, courseID, studentCookies)

	// До прохождения: сертификатов нет, certificateNumber — пустая строка.
	if items := myCertificateItems(t, router, studentCookies); len(items) != 0 {
		t.Fatalf("сертификаты до прохождения: %d, ожидали 0", len(items))
	}
	if got := certificateNumberInMyCourses(t, router, studentCookies, courseID); got != "" {
		t.Fatalf("certificateNumber до прохождения = %q, ожидали пустую строку", got)
	}

	lessons := app.Store.ListLessonsByCourse(courseID)
	if len(lessons) != 1 {
		t.Fatalf("в seeded-курсе %d уроков, ожидали 1", len(lessons))
	}
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessons[0].ID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var completed map[string]any
	decodeBody(t, rec, &completed)
	if completed["percent"] != float64(100) {
		t.Fatalf("после последнего урока percent=%v, ожидали 100", completed["percent"])
	}

	items := myCertificateItems(t, router, studentCookies)
	if len(items) != 1 {
		t.Fatalf("сертификатов после прохождения: %d, ожидали 1", len(items))
	}
	item := items[0]
	number, _ := item["number"].(string)
	if !certificateNumberRe.MatchString(number) {
		t.Fatalf("номер %q не соответствует EDU-<ГГГГ>-<8 hex>", number)
	}
	if item["courseId"] != courseID || item["courseSlug"] != courseSlug {
		t.Fatalf("сертификат не про тот курс: %s", mustJSON(item))
	}
	if title, _ := item["courseTitle"].(string); title == "" {
		t.Fatalf("в сертификате нет courseTitle: %s", mustJSON(item))
	}
	if issued, _ := item["issuedAt"].(string); issued == "" {
		t.Fatalf("в сертификате нет issuedAt: %s", mustJSON(item))
	}

	if got := certificateNumberInMyCourses(t, router, studentCookies, courseID); got != number {
		t.Fatalf("certificateNumber в /me/courses = %q, ожидали %q", got, number)
	}

	// Публичная проверка — без cookie.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/certificates/"+number, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("публичный сертификат: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var public map[string]any
	decodeBody(t, rec, &public)
	if public["number"] != number || public["courseSlug"] != courseSlug {
		t.Fatalf("публичная карточка не совпала: %s", rec.Body.String())
	}
	if public["studentName"] != "Иван Петров" {
		t.Fatalf("studentName=%v, ожидали displayName", public["studentName"])
	}
	if _, hasEmail := public["email"]; hasEmail {
		t.Fatalf("публичная карточка не должна содержать e-mail: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "cert@example.com") {
		t.Fatalf("в публичном ответе утёк e-mail: %s", rec.Body.String())
	}

	// Неизвестный номер — 404 и контрактный текст.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/certificates/EDU-2026-00000000", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("неизвестный сертификат: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var errOut map[string]string
	decodeBody(t, rec, &errOut)
	if errOut["error"] != "Сертификат не найден" {
		t.Fatalf("текст 404: %q", errOut["error"])
	}

	// Список сертификатов — только для вошедших.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/certificates", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me/certificates без входа: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Тест 2: сертификат появляется только после ПОСЛЕДНЕГО урока; повторное
// завершение (повторное достижение 100%) второй сертификат не создаёт.
func TestCertificateOnlyAfterAllLessonsAndIdempotent(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	// Второй урок — video_text: сертификат должен ждать его завершения.
	mods := app.Store.ListModulesByCourse(courseID)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mods[0].ID, "title": "Второй урок", "kind": "video_text", "bodyMd": "текст",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create second lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var second map[string]any
	decodeBody(t, rec, &second)

	studentCookies, _ := registerUser(t, router, "last@example.com", "password123", "")
	enrollFree(t, router, courseID, studentCookies)
	lessons := app.Store.ListLessonsByCourse(courseID)
	if len(lessons) != 2 {
		t.Fatalf("уроков курса: %d, ожидали 2", len(lessons))
	}

	// Первый (quiz) — прогресс 50%, сертификата ещё нет.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessons[0].ID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete first: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var progress map[string]any
	decodeBody(t, rec, &progress)
	if progress["percent"] != float64(50) {
		t.Fatalf("после первого из двух уроков percent=%v, ожидали 50", progress["percent"])
	}
	if items := myCertificateItems(t, router, studentCookies); len(items) != 0 {
		t.Fatalf("сертификат выдан до последнего урока: %d", len(items))
	}
	if got := certificateNumberInMyCourses(t, router, studentCookies, courseID); got != "" {
		t.Fatalf("certificateNumber до последнего урока = %q", got)
	}

	// Последний (video) — 100% и сертификат.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessons[1].ID+"/complete", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete last: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &progress)
	if progress["percent"] != float64(100) {
		t.Fatalf("после последнего урока percent=%v, ожидали 100", progress["percent"])
	}
	items := myCertificateItems(t, router, studentCookies)
	if len(items) != 1 {
		t.Fatalf("сертификатов после 100%%: %d, ожидали 1", len(items))
	}
	number := items[0]["number"].(string)

	// Повторное завершение обоих уроков — сертификат тот же и единственный.
	for _, l := range []struct {
		id   string
		body any
	}{
		{lessons[0].ID, map[string]any{"quizAnswers": map[string]int{"q1": 0}}},
		{lessons[1].ID, nil},
	} {
		rec = doRequest(t, router, http.MethodPost,
			"/api/v1/me/courses/"+courseID+"/lessons/"+l.id+"/complete", l.body, studentCookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("re-complete %s: status=%d body=%s", l.id, rec.Code, rec.Body.String())
		}
	}
	items = myCertificateItems(t, router, studentCookies)
	if len(items) != 1 {
		t.Fatalf("после повторного завершения сертификатов: %d, ожидали 1", len(items))
	}
	if items[0]["number"] != number {
		t.Fatalf("номер изменился: %v → %v", number, items[0]["number"])
	}
	if got := certificateNumberInMyCourses(t, router, studentCookies, courseID); got != number {
		t.Fatalf("certificateNumber после повтора = %q, ожидали %q", got, number)
	}
}

// Тест 3: GET /me/certificates отдаёт новые сертификаты первыми.
func TestMyCertificatesNewestFirst(t *testing.T) {
	app, router := newTestApp()
	_, firstCourseID, _ := seedPublishedCourse(t, app, router, 0)

	// Второй опубликованный курс того же преподавателя — напрямую в store,
	// чтобы не дублировать длинный сценарий модерации.
	teacherID := userIDByEmail(t, app.Store, "teacher@example.com")
	second := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: "second-cert-course", Title: "Второй курс",
		Description: "Курс для проверки порядка сертификатов", PriceCents: 0, Currency: "RUB",
	})
	if !app.Store.SetCourseStatus(second.ID, repo.CoursePublished, "") {
		t.Fatal("не удалось опубликовать второй курс")
	}
	mod := app.Store.CreateModule(second.ID, "Модуль")
	app.Store.CreateLesson(&repo.Lesson{
		ModuleID: mod.ID, Title: "Урок", Kind: repo.LessonVideoText, BodyMD: "текст",
	})

	studentCookies, _ := registerUser(t, router, "order@example.com", "password123", "")
	enrollFree(t, router, firstCourseID, studentCookies)
	enrollFree(t, router, second.ID, studentCookies)

	firstLessons := app.Store.ListLessonsByCourse(firstCourseID)
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+firstCourseID+"/lessons/"+firstLessons[0].ID+"/complete",
		map[string]any{"quizAnswers": map[string]int{"q1": 0}}, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete first course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Разносим issued_at: иначе порядок при равной метке недетерминирован.
	time.Sleep(2 * time.Millisecond)

	secondLessons := app.Store.ListLessonsByCourse(second.ID)
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+second.ID+"/lessons/"+secondLessons[0].ID+"/complete",
		nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete second course: status=%d body=%s", rec.Code, rec.Body.String())
	}

	items := myCertificateItems(t, router, studentCookies)
	if len(items) != 2 {
		t.Fatalf("сертификатов: %d, ожидали 2", len(items))
	}
	if items[0]["courseId"] != second.ID {
		t.Fatalf("первым должен идти новый сертификат (%s), получено: %s",
			second.ID, mustJSON(items))
	}
	if items[1]["courseId"] != firstCourseID {
		t.Fatalf("вторым должен идти старый сертификат (%s), получено: %s",
			firstCourseID, mustJSON(items))
	}
}

// mustJSON — компактный JSON для сообщений об ошибках теста.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<marshal error>"
	}
	return string(b)
}
