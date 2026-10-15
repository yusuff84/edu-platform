package contract_test

// Контрактные тесты отзывов (specs T040; перенос + расширение).
// Ключ контракта: текст отзыва — поле body (не text); рейтинг курса
// приходит в конверте GET /courses/{id}/reviews (расхождение №3: теряет
// его фронтенд-клиент T01, бэкенд отдаёт корректно).

import (
	"net/http"
	"testing"
)

// Отзыв: только после записи, один на ученика, пересчёт рейтинга.
// (перенесено из payments_test.go без изменения логики проверок)
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

// Контракт формы отзыва: ключ body (не text), rating 1..5, список отзывов —
// конверт с avgRating/ratingsCount (расхождение №3 подтверждаем на бэкенде),
// /courses/{id}/reviews принимает и slug.
func TestReviewsContractShapes(t *testing.T) {
	app, router := newTestApp()
	_, courseID, slug := seedPublishedCourse(t, app, router, 0)
	studentCookies, _ := registerUser(t, router, "shapes@example.com", "password123", "")

	// Запись на курс.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID,
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Поле text вместо body — 400 (DisallowUnknownFields).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": 5, "text": "не то поле",
	}, studentCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("отзыв с полем text: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Невалидный рейтинг — 400.
	for _, rating := range []int{0, 6} {
		rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
			"rating": rating,
		}, studentCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("rating=%d: status=%d body=%s", rating, rec.Code, rec.Body.String())
		}
	}

	// Валидный отзыв: 201, форма Review.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
		"rating": 4, "body": "Хороший курс",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rev map[string]any
	decodeBody(t, rec, &rev)
	if rev["studentId"] == "" || rev["rating"] != float64(4) || rev["body"] != "Хороший курс" {
		t.Fatalf("форма Review нарушена: %s", rec.Body.String())
	}

	// Список по slug (backend принимает и UUID, и slug) — конверт с рейтингом.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug+"/reviews", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reviews by slug: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	if list["avgRating"] != float64(4) || list["ratingsCount"] != float64(1) {
		t.Fatalf("конверт отзывов без рейтинга (расхождение №3 закрыто на бэкенде): %s", rec.Body.String())
	}
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v body=%s", list["items"], rec.Body.String())
	}
	if _, hasAuthor := items[0].(map[string]any)["authorEmail"]; hasAuthor {
		t.Fatalf("authorEmail не должен попадать в ответ: %s", rec.Body.String())
	}
}
