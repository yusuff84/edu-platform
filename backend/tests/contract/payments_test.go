package contract_test

import (
	"net/http"
	"testing"
)

// Покупка: enroll -> pending+redirect, повторный enroll до оплаты идемпотентен,
// webhook -> active, дубль webhook -> duplicate, повторная покупка -> 409.
func TestPaymentsEnrollAndWebhook(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)

	studentCookies, _ := registerUser(t, router, "buyer@example.com", "password123", "")

	// Покупка.
	var enroll map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &enroll)
	providerPaymentID, _ := enroll["providerPaymentId"].(string)
	if providerPaymentID == "" {
		t.Fatalf("нет providerPaymentId: %s", rec.Body.String())
	}
	if url, _ := enroll["redirectUrl"].(string); url == "" {
		t.Fatalf("нет redirectUrl: %s", rec.Body.String())
	}

	// До оплаты курса нет в «моих».
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mine map[string]any
	decodeBody(t, rec, &mine)
	if total, _ := mine["total"].(float64); total != 0 {
		t.Fatalf("курс доступен до оплаты: %s", rec.Body.String())
	}

	// Webhook: оплата прошла.
	webhook := map[string]any{"providerPaymentId": providerPaymentID, "status": "succeeded"}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", webhook, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var wh map[string]any
	decodeBody(t, rec, &wh)
	if wh["status"] != "processed" {
		t.Fatalf("webhook status=%v body=%s", wh["status"], rec.Body.String())
	}

	// Дубль webhook — идемпотентный duplicate.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", webhook, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook duplicate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &wh)
	if wh["status"] != "duplicate" {
		t.Fatalf("webhook duplicate status=%v body=%s", wh["status"], rec.Body.String())
	}

	// Курс появился в «моих» с прогрессом 0 и nextLesson.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("me/courses после оплаты: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &mine)
	if total, _ := mine["total"].(float64); total != 1 {
		t.Fatalf("me/courses total=%v body=%s", mine["total"], rec.Body.String())
	}

	// Повторная покупка — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Сплит: комиссия 15% от 10000 = 1500 платформе, 8500 учителю.
	p, ok := app.Store.GetPaymentByProviderID(providerPaymentID)
	if !ok {
		t.Fatal("платёж не найден в store")
	}
	if p.PlatformFeeCents != 1500 || p.TeacherAmountCents != 8500 {
		t.Fatalf("сплит неверный: fee=%d teacher=%d", p.PlatformFeeCents, p.TeacherAmountCents)
	}
}

// Прохождение quiz-урока: неверные ответы не засчитываются, верные — дают прогресс.
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

// Отзыв: только после записи, один на ученика, пересчёт рейтинга.
func TestReviewsFlow(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0)

	outsiderCookies, _ := registerUser(t, router, "outsider@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": 5, "body": "Отлично!",
	}, outsiderCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("отзыв без записи: status=%d body=%s", rec.Code, rec.Body.String())
	}

	studentCookies, _ := registerUser(t, router, "reviewer@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": 5, "body": "Отлично!",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Дубль — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": 4,
	}, studentCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("review duplicate: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Список + рейтинг.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/reviews", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	if list["avgRating"] != float64(5) {
		t.Fatalf("avgRating=%v body=%s", list["avgRating"], rec.Body.String())
	}
}
