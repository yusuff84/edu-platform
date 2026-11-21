package contract_test

// Ручная выдача доступа к курсу администратором (POST /admin/enrollments/grant):
// платный курс без оплаты, повторная выдача, восстановление после отзыва,
// черновик, неизвестный пользователь, права доступа; платёж не создаётся.

import (
	"net/http"
	"testing"

	"edu-platform/backend/internal/repo"
)

func TestAdminGrantEnrollment(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 490000) // платный курс
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	studentCookies, studentID := registerUser(t, router, "gift@example.com", "password123", "")

	grant := func(body map[string]any, cookies []*http.Cookie) (int, map[string]any) {
		t.Helper()
		rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/enrollments/grant", body, cookies)
		out := map[string]any{}
		if rec.Code == http.StatusOK || rec.Header().Get("Content-Type") != "" {
			decodeBody(t, rec, &out)
		}
		return rec.Code, out
	}
	myTotal := func() float64 {
		t.Helper()
		rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
		var mine map[string]any
		decodeBody(t, rec, &mine)
		total, _ := mine["total"].(float64)
		return total
	}

	// Права: аноним 401, ученик 403.
	if code, _ := grant(map[string]any{"email": "gift@example.com", "courseId": courseID}, nil); code != http.StatusUnauthorized {
		t.Fatalf("аноним: %d", code)
	}
	if code, _ := grant(map[string]any{"email": "gift@example.com", "courseId": courseID}, studentCookies); code != http.StatusForbidden {
		t.Fatalf("ученик: %d", code)
	}

	// Выдача платного курса по email (регистр не важен) — без оплаты.
	code, out := grant(map[string]any{"email": "GIFT@example.com", "courseId": courseID}, adminCookies)
	if code != http.StatusOK || out["status"] != "active" || out["created"] != true || out["wasActive"] != false || out["studentId"] != studentID {
		t.Fatalf("grant: %d %v", code, out)
	}
	if got := myTotal(); got != 1 {
		t.Fatalf("/me/courses total=%v, ждали 1", got)
	}
	// Платёж не создан.
	if n := len(app.Store.ListPaymentsByStudent(studentID)); n != 0 {
		t.Fatalf("выдача доступа создала %d платежей", n)
	}

	// Повторная выдача — без изменений.
	if code, out := grant(map[string]any{"studentId": studentID, "courseId": courseID}, adminCookies); code != http.StatusOK || out["wasActive"] != true || out["created"] != false {
		t.Fatalf("повтор: %d %v", code, out)
	}

	// После отзыва выдача восстанавливает доступ.
	if err := app.Store.SetEnrollmentStatus(studentID, courseID, repo.EnrollmentRevoked); err != nil {
		t.Fatal(err)
	}
	if got := myTotal(); got != 0 {
		t.Fatalf("после revoke total=%v", got)
	}
	if code, out := grant(map[string]any{"email": "gift@example.com", "courseId": courseID}, adminCookies); code != http.StatusOK || out["status"] != "active" || out["wasActive"] != false {
		t.Fatalf("после revoke: %d %v", code, out)
	}
	if got := myTotal(); got != 1 {
		t.Fatalf("доступ не восстановлен: total=%v", got)
	}

	// Ошибки: неизвестный email, курс-черновик, пустой курс.
	if code, _ := grant(map[string]any{"email": "nobody@example.com", "courseId": courseID}, adminCookies); code != http.StatusNotFound {
		t.Fatalf("нет пользователя: %d", code)
	}
	if ok := app.Store.SetCourseStatus(courseID, repo.CourseDraft, ""); !ok {
		t.Fatal("SetCourseStatus draft не сработал")
	}
	if code, _ := grant(map[string]any{"email": "gift@example.com", "courseId": courseID}, adminCookies); code != http.StatusBadRequest {
		t.Fatalf("черновик: %d", code)
	}
	if code, _ := grant(map[string]any{"email": "gift@example.com"}, adminCookies); code != http.StatusBadRequest {
		t.Fatalf("без курса: %d", code)
	}
}
