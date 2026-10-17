package contract_test

// Контрактные тесты T16 (п.6): ручное управление доступом к курсу —
// POST /admin/enrollments/{studentId}/courses/{courseId}/revoke|unrevoke.
// До T16 статус EnrollmentRevoked не выставлялся НИКЕМ — админ не мог отозвать
// доступ вручную. Здесь закрепляется форма ответа {studentId,courseId,status}
// и эффект на студента: revoke скрывает курс из /me/courses и блокирует
// .../complete (403), unrevoke возвращает доступ.

import (
	"net/http"
	"testing"
)

// enrollmentPath собирает путь revoke/unrevoke.
func enrollmentPath(action, studentID, courseID string) string {
	return "/api/v1/admin/enrollments/" + studentID + "/courses/" + courseID + "/" + action
}

// TestAdminRevokeUnrevokeLifecycle — полный цикл отзыва доступа:
// покупка → revoke (курс исчезает из /me/courses, complete — 403) →
// unrevoke (доступ возвращается). Плюс права и валидация UUID.
func TestAdminRevokeUnrevokeLifecycle(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Студент покупает курс (webhook активирует запись).
	studentCookies, studentID := registerUser(t, router, "revoked-student@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
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

	// Доступ есть: 1 курс в /me/courses.
	me := func() float64 {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
		var mine map[string]any
		decodeBody(t, rec, &mine)
		total, _ := mine["total"].(float64)
		return total
	}
	if got := me(); got != 1 {
		t.Fatalf("до revoke /me/courses total=%v, ждали 1", got)
	}

	// Аноним и не-админ не могут отзывать доступ.
	if rec := doRequest(t, router, http.MethodPost, enrollmentPath("revoke", studentID, courseID), nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним revoke: status=%d, ждали 401", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodPost, enrollmentPath("revoke", studentID, courseID), nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("студент revoke: status=%d, ждали 403", rec.Code)
	}

	// Revoke: 200, форма ответа контракта, курс скрыт.
	rec = doRequest(t, router, http.MethodPost, enrollmentPath("revoke", studentID, courseID), nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["studentId"] != studentID || out["courseId"] != courseID || out["status"] != "revoked" {
		t.Fatalf("форма ответа revoke нарушена: %s", rec.Body.String())
	}
	if got := me(); got != 0 {
		t.Fatalf("после revoke /me/courses total=%v, ждали 0 (курс скрыт)", got)
	}
	// Complete урока закрыт: 403 (нет активной записи).
	mods := app.Store.ListModulesByCourse(courseID)
	lessons := app.Store.ListLessonsByModule(mods[0].ID)
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessons[0].ID+"/complete", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("complete при revoked: status=%d, ждали 403", rec.Code)
	}

	// Unrevoke: 200, доступ возвращается.
	rec = doRequest(t, router, http.MethodPost, enrollmentPath("unrevoke", studentID, courseID), nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("unrevoke: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["status"] != "active" {
		t.Fatalf("unrevoke должен вернуть status=active: %s", rec.Body.String())
	}
	if got := me(); got != 1 {
		t.Fatalf("после unrevoke /me/courses total=%v, ждали 1", got)
	}
}

// TestAdminRevokeValidationAndNotFound — валидация UUID и 404 для
// несуществующей записи (SetEnrollmentStatus → ErrNotFound).
func TestAdminRevokeValidationAndNotFound(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	_, studentID := registerUser(t, router, "noenroll@example.com", "password123", "")
	_ = app

	// Не-UUID studentId — 400.
	rec := doRequest(t, router, http.MethodPost, enrollmentPath("revoke", "не-uuid", courseID), nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("studentId не UUID: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Не-UUID courseId — 400.
	rec = doRequest(t, router, http.MethodPost, enrollmentPath("revoke", studentID, "не-uuid"), nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("courseId не UUID: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Записи нет — 404 (оба endpoint'а).
	for _, action := range []string{"revoke", "unrevoke"} {
		rec = doRequest(t, router, http.MethodPost, enrollmentPath(action, studentID, courseID), nil, adminCookies)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s без записи: status=%d body=%s", action, rec.Code, rec.Body.String())
		}
	}
	// Несуществующий курс (записи нет) — тоже 404, а не 500.
	rec = doRequest(t, router, http.MethodPost,
		enrollmentPath("revoke", studentID, "00000000-0000-0000-0000-000000000000"), nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужой курс: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminRevokeThenRepurchase — revoke не мешает повторной покупке:
// EnsureEnrollment на успешном платеже возвращает запись в active
// (T16, п.5). Семантика: revoked → (новая оплата) → active.
func TestAdminRevokeThenRepurchase(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	studentCookies, studentID := registerUser(t, router, "repurchase@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	var enroll map[string]any
	decodeBody(t, rec, &enroll)
	doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": enroll["providerPaymentId"], "status": "succeeded",
	}, nil)

	// Админ отзывает доступ.
	if rec := doRequest(t, router, http.MethodPost, enrollmentPath("revoke", studentID, courseID), nil, adminCookies); rec.Code != http.StatusOK {
		t.Fatalf("revoke: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Повторная покупка при revoked: не 409 (там проверяется только active),
	// создаётся новый платёж.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("покупка после revoke: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &enroll)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": enroll["providerPaymentId"], "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Доступ возвращён без участия админа — EnsureEnrollment реанимировал запись.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies)
	var mine map[string]any
	decodeBody(t, rec, &mine)
	if total, _ := mine["total"].(float64); total != 1 {
		t.Fatalf("после repurchase /me/courses total=%v, ждали 1: %s", mine["total"], rec.Body.String())
	}
	// Статус в сторе — active, а не revoked.
	e, ok := app.Store.GetEnrollment(studentID, courseID)
	if !ok || e.Status != "active" {
		t.Fatalf("запись должна стать active: %+v", e)
	}
}
