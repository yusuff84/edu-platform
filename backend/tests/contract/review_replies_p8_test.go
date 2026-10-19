package contract_test

// T69 — ответы преподавателя на отзывы и разбивка оценок (волна P8).
// Контракт — Task/00-api-contract.md, раздел «T69 — ответы преподавателя на
// отзывы + разбивка оценок» (заморожен):
//   - PUT/DELETE /teacher/courses/{id}/reviews/{reviewId}/reply (владелец/admin);
//   - Review во всех видах ответов получает teacherReply/teacherReplyAt;
//   - GET /courses/{id}/reviews?rating=1..5 фильтрует items/total, а
//     avgRating/ratingsCount/breakdown считаются по всем отзывам курса.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// enrollAndReview записывает ученика на бесплатный курс и оставляет отзыв.
// Возвращает id отзыва. Заодно проверяет аддитивные поля T69 в ответе POST.
func enrollAndReview(t *testing.T, router http.Handler, courseID, email string, rating int, body string) string {
	t.Helper()
	cookies, _ := registerUser(t, router, email, "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll %s: status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": rating, "body": body,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review %s: status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	var rev map[string]any
	decodeBody(t, rec, &rev)
	if rev["teacherReply"] != "" {
		t.Fatalf("новый отзыв с teacherReply=%v, ждали пустую строку: %s", rev["teacherReply"], rec.Body.String())
	}
	if at, has := rev["teacherReplyAt"]; !has || at != nil {
		t.Fatalf("новый отзыв: teacherReplyAt=%v (поле есть=%v), ждали null: %s", at, has, rec.Body.String())
	}
	id, _ := rev["id"].(string)
	if id == "" {
		t.Fatalf("POST review без id: %s", rec.Body.String())
	}
	return id
}

// reviewByID достаёт отзыв из публичного списка по id (items всегда массив).
func reviewByID(t *testing.T, router http.Handler, courseID, reviewID string) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/reviews", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	items, ok := list["items"].([]any)
	if !ok {
		t.Fatalf("items не массив: %s", rec.Body.String())
	}
	for _, raw := range items {
		rv, _ := raw.(map[string]any)
		if rv["id"] == reviewID {
			return rv
		}
	}
	t.Fatalf("отзыв %s не найден в списке: %s", reviewID, rec.Body.String())
	return nil
}

// TestTeacherReviewReplyLifecycle — создание/замена/удаление ответа, права
// (403), 404 для отзыва другого курса, валидация длины, teacherReply в ответах.
func TestTeacherReviewReplyLifecycle(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	reviewID := enrollAndReview(t, router, courseID, "reply-student@example.com", 4, "Хороший курс")
	replyPath := "/api/v1/teacher/courses/" + courseID + "/reviews/" + reviewID + "/reply"

	// Изначально ответа нет, поля присутствуют в публичном списке.
	rv := reviewByID(t, router, courseID, reviewID)
	if rv["teacherReply"] != "" || rv["teacherReplyAt"] != nil {
		t.Fatalf("до ответа teacherReply=%v teacherReplyAt=%v: ждали пустую строку/null",
			rv["teacherReply"], rv["teacherReplyAt"])
	}

	// Чужой преподаватель — 403 (курс ему не принадлежит).
	registerUser(t, router, "reply-other-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "reply-other-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher чужому преподавателю")
	}
	otherCookies := loginUser(t, router, "reply-other-teacher@example.com", "password123")
	rec := doRequest(t, router, http.MethodPut, replyPath, map[string]any{"body": "Чужой ответ"}, otherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой преподаватель: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Валидация длины: после trim 3..2000 символов.
	for _, bad := range []string{"", "  ab  ", strings.Repeat("я", 2001)} {
		rec = doRequest(t, router, http.MethodPut, replyPath, map[string]any{"body": bad}, teacherCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body len=%d: status=%d body=%s", len(bad), rec.Code, rec.Body.String())
		}
		var errBody map[string]string
		decodeBody(t, rec, &errBody)
		if errBody["error"] != "Ответ: от 3 до 2000 символов" {
			t.Fatalf("текст 400=%q: %s", errBody["error"], rec.Body.String())
		}
	}
	// Границы принимаются: ровно 3 символа.
	rec = doRequest(t, router, http.MethodPut, replyPath, map[string]any{"body": "  да!  "}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("body из 3 символов после trim: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Установка: 200 {"review": ...}, текст обрезан, время проставлено.
	rec = doRequest(t, router, http.MethodPut, replyPath, map[string]any{
		"body": "  Спасибо за отзыв!  ",
	}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT reply: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	decodeBody(t, rec, &envelope)
	created, _ := envelope["review"].(map[string]any)
	if created["teacherReply"] != "Спасибо за отзыв!" {
		t.Fatalf("teacherReply=%v (trim?): %s", created["teacherReply"], rec.Body.String())
	}
	firstAt, ok := created["teacherReplyAt"].(string)
	if !ok || firstAt == "" {
		t.Fatalf("teacherReplyAt не строка: %s", rec.Body.String())
	}
	firstTime, err := time.Parse(time.RFC3339Nano, firstAt)
	if err != nil {
		t.Fatalf("teacherReplyAt не RFC3339: %v (%s)", err, firstAt)
	}

	// Ответ виден публично.
	rv = reviewByID(t, router, courseID, reviewID)
	if rv["teacherReply"] != "Спасибо за отзыв!" || rv["teacherReplyAt"] == nil {
		t.Fatalf("ответ не виден в публичном списке: %v", rv)
	}

	// Повторный PUT заменяет текст и обновляет teacherReplyAt.
	time.Sleep(2 * time.Millisecond)
	rec = doRequest(t, router, http.MethodPut, replyPath, map[string]any{
		"body": "Обновлённый ответ",
	}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("повторный PUT: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &envelope)
	updated, _ := envelope["review"].(map[string]any)
	if updated["teacherReply"] != "Обновлённый ответ" {
		t.Fatalf("текст ответа не заменён: %s", rec.Body.String())
	}
	secondAt, _ := updated["teacherReplyAt"].(string)
	secondTime, err := time.Parse(time.RFC3339Nano, secondAt)
	if err != nil {
		t.Fatalf("teacherReplyAt после замены не RFC3339: %v (%s)", err, secondAt)
	}
	if !secondTime.After(firstTime) {
		t.Fatalf("teacherReplyAt не обновился: было %s, стало %s", firstAt, secondAt)
	}

	// 404: отзыв другого курса и несуществующий отзыв.
	otherTeacherCookies, otherCourseID := seedTeacherCourse(t, app, router, "reply-foreign")
	rec = doRequest(t, router, http.MethodPut,
		"/api/v1/teacher/courses/"+otherCourseID+"/reviews/"+reviewID+"/reply",
		map[string]any{"body": "Ответ не тому курсу"}, otherTeacherCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("отзыв другого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var errBody map[string]string
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Отзыв не найден" {
		t.Fatalf("текст 404=%q: %s", errBody["error"], rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPut,
		"/api/v1/teacher/courses/"+courseID+"/reviews/00000000-0000-0000-0000-000000000000/reply",
		map[string]any{"body": "Нет такого отзыва"}, teacherCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("несуществующий отзыв: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// DELETE: 204, повторный DELETE тоже 204, ответ очищен.
	rec = doRequest(t, router, http.MethodDelete, replyPath, nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE reply: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rv = reviewByID(t, router, courseID, reviewID)
	if rv["teacherReply"] != "" || rv["teacherReplyAt"] != nil {
		t.Fatalf("после DELETE teacherReply=%v teacherReplyAt=%v", rv["teacherReply"], rv["teacherReplyAt"])
	}
	rec = doRequest(t, router, http.MethodDelete, replyPath, nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("повторный DELETE: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// DELETE: 403 чужому, 404 для отзыва другого курса.
	rec = doRequest(t, router, http.MethodDelete, replyPath, nil, otherTeacherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE чужой курс: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+otherCourseID+"/reviews/"+reviewID+"/reply", nil, otherTeacherCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE отзыв другого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestReviewReplyAdminAllowed — admin может ответить на отзыв любого курса
// (контракт: владелец/admin).
func TestReviewReplyAdminAllowed(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0)
	reviewID := enrollAndReview(t, router, courseID, "reply-admin-student@example.com", 5, "Отлично")
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	rec := doRequest(t, router, http.MethodPut,
		"/api/v1/teacher/courses/"+courseID+"/reviews/"+reviewID+"/reply",
		map[string]any{"body": "Ответ администратора"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT reply admin: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	decodeBody(t, rec, &envelope)
	if rev, _ := envelope["review"].(map[string]any); rev["teacherReply"] != "Ответ администратора" {
		t.Fatalf("admin-ответ не сохранён: %s", rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/reviews/"+reviewID+"/reply", nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE reply admin: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestReviewsRatingFilterAndBreakdown — фильтр по оценке и breakdown:
// items/total по фильтру, avgRating/ratingsCount/breakdown — по всем отзывам;
// невалидный rating → 400 с контрактным текстом.
func TestReviewsRatingFilterAndBreakdown(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0)

	// Оценки: 5, 4, 4, 2. Среднее 15/4 = 3.75 → 3.8 (округление до 1 знака).
	enrollAndReview(t, router, courseID, "filter-s1@example.com", 5, "Пять")
	enrollAndReview(t, router, courseID, "filter-s2@example.com", 4, "Четыре")
	enrollAndReview(t, router, courseID, "filter-s3@example.com", 4, "")
	enrollAndReview(t, router, courseID, "filter-s4@example.com", 2, "Два")

	checkBreakdown := func(list map[string]any) {
		t.Helper()
		breakdown, ok := list["breakdown"].(map[string]any)
		if !ok {
			t.Fatalf("breakdown не объект: %v", list["breakdown"])
		}
		if len(breakdown) != 5 {
			t.Fatalf("в breakdown %d ключей, ждали 5: %v", len(breakdown), breakdown)
		}
		want := map[string]float64{"1": 0, "2": 1, "3": 0, "4": 2, "5": 1}
		for key, n := range want {
			if breakdown[key] != n {
				t.Fatalf("breakdown[%q]=%v, ждали %v: %v", key, breakdown[key], n, breakdown)
			}
		}
	}

	// Без фильтра: все 4 отзыва, рейтинг и breakdown по всем.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/reviews", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews all: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var all map[string]any
	decodeBody(t, rec, &all)
	if all["total"] != float64(4) {
		t.Fatalf("total=%v, ждали 4: %s", all["total"], rec.Body.String())
	}
	if items, _ := all["items"].([]any); len(items) != 4 {
		t.Fatalf("items=%d, ждали 4: %s", len(items), rec.Body.String())
	}
	if all["avgRating"] != 3.8 || all["ratingsCount"] != float64(4) {
		t.Fatalf("avgRating/ratingsCount=%v/%v, ждали 3.8/4: %s",
			all["avgRating"], all["ratingsCount"], rec.Body.String())
	}
	checkBreakdown(all)

	// rating=4: items/total по фильтру, агрегаты не меняются.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/reviews?rating=4", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews rating=4: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var four map[string]any
	decodeBody(t, rec, &four)
	if four["total"] != float64(2) {
		t.Fatalf("total(rating=4)=%v, ждали 2: %s", four["total"], rec.Body.String())
	}
	items, _ := four["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items(rating=4)=%d, ждали 2: %s", len(items), rec.Body.String())
	}
	for _, raw := range items {
		if rv, _ := raw.(map[string]any); rv["rating"] != float64(4) {
			t.Fatalf("в фильтре rating=4 чужой отзыв: %v", rv)
		}
	}
	if four["avgRating"] != 3.8 || four["ratingsCount"] != float64(4) {
		t.Fatalf("фильтр повлиял на avgRating/ratingsCount: %v/%v: %s",
			four["avgRating"], four["ratingsCount"], rec.Body.String())
	}
	checkBreakdown(four)

	// rating=1: пустой массив (не null), агрегаты и breakdown прежние.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/reviews?rating=1", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews rating=1: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var none map[string]any
	decodeBody(t, rec, &none)
	if none["total"] != float64(0) {
		t.Fatalf("total(rating=1)=%v, ждали 0: %s", none["total"], rec.Body.String())
	}
	if items, ok := none["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("items(rating=1)=%v, ждали пустой массив: %s", none["items"], rec.Body.String())
	}
	if none["avgRating"] != 3.8 || none["ratingsCount"] != float64(4) {
		t.Fatalf("пустой фильтр повлиял на агрегаты: %s", rec.Body.String())
	}
	checkBreakdown(none)

	// Невалидный rating → 400 с контрактным текстом.
	for _, raw := range []string{"0", "6", "-1", "abc", "2.5"} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/reviews?rating="+raw, nil, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("rating=%s: status=%d body=%s", raw, rec.Code, rec.Body.String())
		}
		var errBody map[string]string
		decodeBody(t, rec, &errBody)
		if errBody["error"] != "Оценка должна быть числом от 1 до 5" {
			t.Fatalf("rating=%s: текст 400=%q", raw, errBody["error"])
		}
	}
}

// TestReviewReplyInTeacherAndAdminViews — teacherReply приходит в админ-списке
// отзывов курса (аддитивные поля T69), а teacher-статистика по-прежнему
// корректно считает reviewsCount/avgRating.
func TestReviewReplyInTeacherAndAdminViews(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)
	reviewID := enrollAndReview(t, router, courseID, "reply-views@example.com", 5, "Супер")
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	rec := doRequest(t, router, http.MethodPut,
		"/api/v1/teacher/courses/"+courseID+"/reviews/"+reviewID+"/reply",
		map[string]any{"body": "Спасибо, рад стараться"}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT reply: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Админский список отзывов курса: поля ответа добавлены аддитивно.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID+"/reviews", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin reviews: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var adminList struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &adminList)
	if len(adminList.Items) != 1 {
		t.Fatalf("admin items=%d: %s", len(adminList.Items), rec.Body.String())
	}
	if adminList.Items[0]["teacherReply"] != "Спасибо, рад стараться" || adminList.Items[0]["teacherReplyAt"] == nil {
		t.Fatalf("ответ не виден в админ-списке: %s", rec.Body.String())
	}

	// Teacher-статистика по-прежнему считает отзывы и рейтинг.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/stats", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher stats: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stats map[string]any
	decodeBody(t, rec, &stats)
	if stats["reviewsCount"] != float64(1) || stats["avgRating"] != float64(5) {
		t.Fatalf("teacher stats: %s", rec.Body.String())
	}
}
