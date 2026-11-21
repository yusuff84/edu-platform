package contract_test

// Расширенная админка (handler/admin_ops.go): пользователи, пароли, прогресс,
// сертификаты, публикация и владелец курса, тексты сайта; права — только admin.

import (
	"net/http"
	"testing"
)

func TestAdminOps(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 490000)
	_ = teacherCookies
	admin := loginUser(t, router, "admin@example.com", "password123")
	studentCookies, studentID := registerUser(t, router, "ops-student@example.com", "password123", "")

	// Не-админ — 403 на любую операцию.
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "x@example.com"}, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("student create user: %d", rec.Code)
	}

	// Создание пользователя со сгенерированным паролем — им можно войти.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "New.User@example.com", "role": "teacher", "displayName": "Новый Преподаватель"}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		User     struct{ ID, Email, Role string } `json:"user"`
		Password string                           `json:"password"`
	}
	decodeBody(t, rec, &created)
	if created.Password == "" || created.User.Email != "new.user@example.com" || created.User.Role != "teacher" {
		t.Fatalf("create user body: %+v", created)
	}
	loginUser(t, router, "new.user@example.com", created.Password)
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "new.user@example.com"}, admin); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate user: %d", rec.Code)
	}

	// Смена пароля админом: старая сессия ученика завершается, новый пароль работает.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/users/"+studentID+"/password", map[string]any{"password": "brand-new-pass"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set password: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies); rec.Code != http.StatusUnauthorized {
		t.Fatalf("старая сессия должна быть завершена: %d", rec.Code)
	}
	studentCookies = loginUser(t, router, "ops-student@example.com", "brand-new-pass")

	// Профиль: email и имя; занятый email — 409.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/profile", map[string]any{"displayName": "Иван Петров", "email": "ivan@example.com"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/profile", map[string]any{"email": "new.user@example.com"}, admin); rec.Code != http.StatusConflict {
		t.Fatalf("email conflict: %d", rec.Code)
	}

	// Засчитать курс целиком → сертификат, курс в списке пользователя с прогрессом.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/enrollments/"+studentID+"/courses/"+courseID+"/complete", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body.String())
	}
	var comp map[string]string
	decodeBody(t, rec, &comp)
	number := comp["certificateNumber"]
	if number == "" {
		t.Fatal("нет номера сертификата")
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/users/"+studentID+"/courses", nil, admin)
	var uc struct {
		Items []struct {
			CompletedLessons, TotalLessons int
			CertificateNumber            string
		} `json:"items"`
	}
	decodeBody(t, rec, &uc)
	if len(uc.Items) != 1 || uc.Items[0].CompletedLessons != uc.Items[0].TotalLessons || uc.Items[0].CertificateNumber != number {
		t.Fatalf("user courses: %+v", uc)
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/certificates/"+number, nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("публичная страница сертификата: %d", rec.Code)
	}

	// Список сертификатов с поиском, отзыв сертификата.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/certificates?q=ivan", nil, admin)
	var certs struct {
		Items []struct{ Number, Email, StudentName string } `json:"items"`
	}
	decodeBody(t, rec, &certs)
	if len(certs.Items) != 1 || certs.Items[0].StudentName != "Иван Петров" {
		t.Fatalf("certificates: %+v", certs)
	}
	if rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/certificates/"+number, nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke cert: %d", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/certificates/"+number, nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("отозванный сертификат: %d", rec.Code)
	}

	// Сброс прогресса.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/enrollments/"+studentID+"/courses/"+courseID+"/reset-progress", nil, admin)
	var reset map[string]int
	decodeBody(t, rec, &reset)
	if rec.Code != http.StatusOK || reset["deleted"] == 0 {
		t.Fatalf("reset: %d %v", rec.Code, reset)
	}

	// Выдача сертификата вручную по email.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/certificates", map[string]any{"email": "ivan@example.com", "courseId": courseID}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("issue cert: %d %s", rec.Code, rec.Body.String())
	}

	// Передача курса: студенту нельзя, преподавателю можно.
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/owner", map[string]any{"email": "ivan@example.com"}, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("owner student: %d", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/owner", map[string]any{"email": "new.user@example.com"}, admin); rec.Code != http.StatusOK {
		t.Fatalf("owner teacher: %d %s", rec.Code, rec.Body.String())
	}
	if c, _ := app.Store.GetCourseByID(courseID); c.TeacherID != created.User.ID {
		t.Fatal("владелец не сменился")
	}

	// Тексты сайта: сохраняются и попадают в публичные настройки; неизвестный ключ — 400.
	rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/site-settings", map[string]any{"settings": map[string]string{"contact_phone": "+7 999 000-00-00", "promo_text": "Новый курс!"}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("site settings: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPut, "/api/v1/admin/site-settings", map[string]any{"settings": map[string]string{"hack": "x"}}, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown key: %d", rec.Code)
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/settings/public", nil, nil)
	var pub struct {
		Site map[string]string `json:"site"`
	}
	decodeBody(t, rec, &pub)
	if pub.Site["contact_phone"] != "+7 999 000-00-00" || pub.Site["promo_text"] != "Новый курс!" {
		t.Fatalf("public settings: %+v", pub)
	}
}
