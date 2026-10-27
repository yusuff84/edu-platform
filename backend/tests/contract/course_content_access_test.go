package contract_test

// H1 (P11): доступ к содержимому уроков на публичной странице курса.
// Гость видит bodyMd/videoUrl/liveUrl только у превью-уроков; активная
// запись открывает контент всех уроков, но не ответы тестов; полный вид
// quiz/codeTask — только у владельца и админа. Заблокированный пользователь
// считается гостем. M2: отзывы неопубликованного курса гостю и студенту
// недоступны (404 «Курс не найден»).

import (
	"net/http"
	"strings"
	"testing"

	"edu-platform/backend/internal/repo"
)

// seedContentAccessCourse добавляет в черновой курс превью-урок и закрытые
// уроки всех видов, затем публикует курс. Курс платный: fixture создаёт его
// с priceCents=1000.
func seedContentAccessCourse(t *testing.T, f *previewFixture) (previewID, closedVideoID, closedLiveID, quizID, codeID string) {
	t.Helper()
	lessonID := func(l map[string]any) string {
		t.Helper()
		id, _ := l["id"].(string)
		if id == "" {
			t.Fatalf("урок создан без id: %s", mustJSON(l))
		}
		return id
	}

	previewID = lessonID(f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Превью-урок", "kind": "video_text",
		"bodyMd": "текст превью", "videoUrl": "https://video.example/preview",
		"isPreview": true,
	}))
	closedVideoID = lessonID(f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Закрытый видеоурок", "kind": "video_text",
		"bodyMd": "закрытый текст", "videoUrl": "https://video.example/closed",
	}))
	closedLiveID = lessonID(f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Закрытый эфир", "kind": "live",
		"liveUrl": "https://live.example/closed",
	}))
	quizID = lessonID(f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Закрытый тест", "kind": "quiz",
		"quiz": map[string]any{"questions": []any{map[string]any{
			"id": "q1", "text": "2+2?", "options": []string{"3", "4"}, "correct": 1,
			"explanation": "Потому что четыре",
		}}},
	}))
	codeID = lessonID(f.postLesson(t, f.courseID, f.moduleID, map[string]any{
		"title": "Закрытое задание", "kind": "code",
		"codeTask": map[string]any{
			"language": "python", "template": "print(1)",
			"tests": []any{
				map[string]any{"name": "visibleTest", "input": "1", "expected": "1"},
				map[string]any{"name": "hiddenSecret", "input": "2", "expected": "2", "hidden": true},
			},
		},
	}))

	f.publishCourse(t, f.courseID)
	return previewID, closedVideoID, closedLiveID, quizID, codeID
}

// requireLessonField проверяет строковое поле урока в публичном ответе.
func requireLessonField(t *testing.T, lesson map[string]any, field, want string) {
	t.Helper()
	got, ok := lesson[field].(string)
	if !ok || got != want {
		t.Fatalf("поле %s=%v, ждали %q: %s", field, lesson[field], want, mustJSON(lesson))
	}
}

// requireLessonFieldAbsent проверяет, что поле урока не заполнено (omitempty).
func requireLessonFieldAbsent(t *testing.T, lesson map[string]any, field string) {
	t.Helper()
	if v, ok := lesson[field]; ok {
		t.Fatalf("поле %s не должно отдаваться: %v (%s)", field, v, mustJSON(lesson))
	}
}

// TestCourseContentAccessLevels — гость, активная запись, владелец и админ
// на одной опубликованной странице курса.
func TestCourseContentAccessLevels(t *testing.T) {
	f := newPreviewFixture(t)
	previewID, closedVideoID, closedLiveID, quizID, codeID := seedContentAccessCourse(t, f)

	// 1. Гость: контент только у превью; тесты и задания — без ответов.
	rec := doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("гость: status=%d body=%s", rec.Code, rec.Body.String())
	}
	guestBody := rec.Body.String()
	guestPreview := lessonFromPublicPage(t, rec.Body.Bytes(), previewID)
	requireLessonField(t, guestPreview, "bodyMd", "текст превью")
	requireLessonField(t, guestPreview, "videoUrl", "https://video.example/preview")
	requireLessonFieldAbsent(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedVideoID), "bodyMd")
	requireLessonFieldAbsent(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedVideoID), "videoUrl")
	requireLessonFieldAbsent(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedLiveID), "liveUrl")
	if strings.Contains(guestBody, `"correct"`) {
		t.Fatalf("гость видит правильные ответы теста: %s", guestBody)
	}
	if strings.Contains(guestBody, "hiddenSecret") {
		t.Fatalf("гость видит скрытый тест задания: %s", guestBody)
	}
	if !strings.Contains(guestBody, "visibleTest") {
		t.Fatalf("гость не видит видимый тест задания: %s", guestBody)
	}
	if _, ok := lessonFromPublicPage(t, rec.Body.Bytes(), quizID)["quiz"].(map[string]any); !ok {
		t.Fatalf("у гостя нет публичного вида quiz: %s", guestBody)
	}
	if _, ok := lessonFromPublicPage(t, rec.Body.Bytes(), codeID)["codeTask"].(map[string]any); !ok {
		t.Fatalf("у гостя нет публичного вида codeTask: %s", guestBody)
	}

	// 2. Активная запись: контент у всех уроков, но ответы всё ещё скрыты.
	studentCookies, studentID := registerUser(t, f.router, "content-student@example.com", "password123", "")
	if _, created := f.app.Store.EnsureEnrollment(studentID, f.courseID); !created {
		t.Fatal("не удалось создать активную запись студента")
	}
	rec = doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("ученик: status=%d body=%s", rec.Code, rec.Body.String())
	}
	studentBody := rec.Body.String()
	requireLessonField(t, lessonFromPublicPage(t, rec.Body.Bytes(), previewID), "bodyMd", "текст превью")
	requireLessonField(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedVideoID), "bodyMd", "закрытый текст")
	requireLessonField(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedVideoID), "videoUrl", "https://video.example/closed")
	requireLessonField(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedLiveID), "liveUrl", "https://live.example/closed")
	if strings.Contains(studentBody, `"correct"`) {
		t.Fatalf("ученик видит правильные ответы теста: %s", studentBody)
	}
	if strings.Contains(studentBody, "hiddenSecret") {
		t.Fatalf("ученик видит скрытый тест задания: %s", studentBody)
	}
	if !strings.Contains(studentBody, "visibleTest") {
		t.Fatalf("ученик не видит видимый тест задания: %s", studentBody)
	}

	// 3. Владелец: контент всех уроков + полный quiz/codeTask.
	rec = doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, f.teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("владелец: status=%d body=%s", rec.Code, rec.Body.String())
	}
	ownerBody := rec.Body.String()
	requireLessonField(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedVideoID), "bodyMd", "закрытый текст")
	if !strings.Contains(ownerBody, `"correct":1`) {
		t.Fatalf("владелец не видит правильный ответ теста: %s", ownerBody)
	}
	if !strings.Contains(ownerBody, "hiddenSecret") {
		t.Fatalf("владелец не видит скрытый тест задания: %s", ownerBody)
	}

	// 4. Админ: тот же полный вид, что и у владельца.
	rec = doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, f.adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("админ: status=%d body=%s", rec.Code, rec.Body.String())
	}
	adminBody := rec.Body.String()
	if !strings.Contains(adminBody, `"correct":1`) {
		t.Fatalf("админ не видит правильный ответ теста: %s", adminBody)
	}
	if !strings.Contains(adminBody, "hiddenSecret") {
		t.Fatalf("админ не видит скрытый тест задания: %s", adminBody)
	}
}

// TestBlockedUserGetsGuestView — заблокированный владелец не получает
// полный вид и не видит свой неопубликованный курс.
func TestBlockedUserGetsGuestView(t *testing.T) {
	f := newPreviewFixture(t)
	previewID, closedVideoID, _, _, _ := seedContentAccessCourse(t, f)

	// Черновик создаём до блокировки: после неё teacher API недоступен.
	_, draftSlug, _ := f.createCourse(t, "Черновик заблокированного")

	teacherID := userIDByEmail(t, f.app.Store, "preview-teacher@example.com")
	if !f.app.Store.SetUserStatus(teacherID, repo.UserBlocked) {
		t.Fatal("не удалось заблокировать владельца курса")
	}

	rec := doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+f.slug, nil, f.teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("заблокированный на published: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	requireLessonField(t, lessonFromPublicPage(t, rec.Body.Bytes(), previewID), "bodyMd", "текст превью")
	requireLessonFieldAbsent(t, lessonFromPublicPage(t, rec.Body.Bytes(), closedVideoID), "bodyMd")
	if strings.Contains(body, `"correct"`) {
		t.Fatalf("заблокированный видит правильные ответы теста: %s", body)
	}
	if strings.Contains(body, "hiddenSecret") {
		t.Fatalf("заблокированный видит скрытый тест задания: %s", body)
	}

	rec = doRequest(t, f.router, http.MethodGet, "/api/v1/courses/"+draftSlug, nil, f.teacherCookies)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Курс не найден") {
		t.Fatalf("заблокированный видит черновик: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestReviewsOfUnpublishedCourseAreHidden — M2: отзывы черновика получают
// только владелец и админ; гостю и студенту — 404 «Курс не найден».
func TestReviewsOfUnpublishedCourseAreHidden(t *testing.T) {
	f := newPreviewFixture(t)
	studentCookies, _ := registerUser(t, f.router, "reviews-hidden-student@example.com", "password123", "")

	for _, tc := range []struct {
		name    string
		path    string
		cookies []*http.Cookie
	}{
		{"гость по id", "/api/v1/courses/" + f.courseID + "/reviews", nil},
		{"гость по slug", "/api/v1/courses/" + f.slug + "/reviews", nil},
		{"студент", "/api/v1/courses/" + f.courseID + "/reviews", studentCookies},
	} {
		rec := doRequest(t, f.router, http.MethodGet, tc.path, nil, tc.cookies)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Курс не найден") {
			t.Fatalf("%s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
		}
	}

	for _, tc := range []struct {
		name    string
		cookies []*http.Cookie
	}{
		{"владелец", f.teacherCookies},
		{"админ", f.adminCookies},
	} {
		rec := doRequest(t, f.router, http.MethodGet,
			"/api/v1/courses/"+f.courseID+"/reviews", nil, tc.cookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
		}
	}
}
