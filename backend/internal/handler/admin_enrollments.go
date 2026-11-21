package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// RegisterAdminEnrollmentRoutes регистрирует админ-эндпоинты ручного
// управления доступом (T16, п.6):
//
//	POST /admin/enrollments/{studentId}/courses/{courseId}/revoke
//	POST /admin/enrollments/{studentId}/courses/{courseId}/unrevoke
//
// Как и RegisterAdminPaymentsRoutes (admin_payments.go), метод регистрирует
// АБСОЛЮТНЫЕ пути, поэтому вызывается на уровне /api/v1 в middleware.go,
// а НЕ внутри r.Route("/admin", ...) — иначе получилось бы
// /admin/admin/enrollments/... . requireAuth + requireRole(admin) вешаются
// здесь же через .With.
func (a *App) RegisterAdminEnrollmentRoutes(r chi.Router) {
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Post("/admin/enrollments/{studentId}/courses/{courseId}/revoke", a.AdminRevokeEnrollment)
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Post("/admin/enrollments/{studentId}/courses/{courseId}/unrevoke", a.AdminUnrevokeEnrollment)
	// Ручная выдача доступа к курсу (в т.ч. платному) без оплаты.
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Post("/admin/enrollments/grant", a.AdminGrantEnrollment)
}

// enrollmentStatusItem — ответ смены статуса записи на курсе.
type enrollmentStatusItem struct {
	StudentID string                `json:"studentId"`
	CourseID  string                `json:"courseId"`
	Status    repo.EnrollmentStatus `json:"status"`
}

// AdminRevokeEnrollment — POST .../revoke: отзывает доступ ученика к курсу
// (EnrollmentRevoked). Ревизионный статус выставляется только здесь: возврат
// средств ставит refunded (webhook), повторная покупка — active (EnsureEnrollment).
func (a *App) AdminRevokeEnrollment(w http.ResponseWriter, r *http.Request) {
	a.setEnrollmentStatus(w, r, repo.EnrollmentRevoked)
}

// AdminUnrevokeEnrollment — POST .../unrevoke: возвращает доступ
// (EnrollmentActive), например если отзыв был сделан ошибочно.
func (a *App) AdminUnrevokeEnrollment(w http.ResponseWriter, r *http.Request) {
	a.setEnrollmentStatus(w, r, repo.EnrollmentActive)
}

// setEnrollmentStatus общая реализация для revoke/unrevoke: валидирует UUID
// из пути и переводит запись ученика на курс в новый статус.
func (a *App) setEnrollmentStatus(w http.ResponseWriter, r *http.Request, status repo.EnrollmentStatus) {
	// Страховка к requireRole: маршруты безопасны даже вне ожидаемой цепочки.
	if !a.adminOnly(w, r) {
		return
	}
	unlock, locked := a.lockBilling(w, r)
	if !locked {
		return
	}
	defer unlock()
	studentID := strings.TrimSpace(chi.URLParam(r, "studentId"))
	courseID := strings.TrimSpace(chi.URLParam(r, "courseId"))
	if _, err := uuid.Parse(studentID); err != nil {
		writeErr(w, http.StatusBadRequest, "Параметр studentId должен быть UUID")
		return
	}
	if _, err := uuid.Parse(courseID); err != nil {
		writeErr(w, http.StatusBadRequest, "Параметр courseId должен быть UUID")
		return
	}
	if err := a.Store.SetEnrollmentStatus(studentID, courseID, status); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Запись ученика на курс не найдена")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось изменить статус записи на курс")
		return
	}
	writeJSON(w, http.StatusOK, enrollmentStatusItem{
		StudentID: studentID, CourseID: courseID, Status: status,
	})
}

// grantEnrollmentRequest — тело POST /admin/enrollments/grant: ученик по
// email или id и курс по id.
type grantEnrollmentRequest struct {
	Email     string `json:"email"`
	StudentID string `json:"studentId"`
	CourseID  string `json:"courseId"`
}

// grantEnrollmentResponse — итог выдачи доступа.
type grantEnrollmentResponse struct {
	StudentID   string                `json:"studentId"`
	Email       string                `json:"email"`
	CourseID    string                `json:"courseId"`
	CourseTitle string                `json:"courseTitle"`
	Status      repo.EnrollmentStatus `json:"status"`
	// Created — запись создана сейчас; false — доступ уже был или восстановлен.
	Created bool `json:"created"`
	// WasActive — у ученика уже был активный доступ, ничего не изменилось.
	WasActive bool `json:"wasActive"`
}

// AdminGrantEnrollment — POST /admin/enrollments/grant: администратор
// открывает ученику доступ к курсу без оплаты (подарок, партнёрский доступ,
// оплата вне платформы). Работает и для платных курсов; отозванный или
// возвращённый доступ снова становится активным. Платёж не создаётся —
// в журнале платежей и выручке такая запись не появляется.
// 200 grantEnrollmentResponse; 400 — нет курса/ученика в запросе или курс
// ещё черновик; 404 — нет пользователя или курса.
func (a *App) AdminGrantEnrollment(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	var req grantEnrollmentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	courseID := strings.TrimSpace(req.CourseID)
	if _, err := uuid.Parse(courseID); err != nil {
		writeErr(w, http.StatusBadRequest, "Выберите курс")
		return
	}
	var (
		student *repo.User
		found   bool
	)
	switch {
	case strings.TrimSpace(req.StudentID) != "":
		if _, err := uuid.Parse(strings.TrimSpace(req.StudentID)); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр studentId должен быть UUID")
			return
		}
		student, found = a.Store.GetUserByID(strings.TrimSpace(req.StudentID))
	case strings.TrimSpace(req.Email) != "":
		student, found = a.Store.GetUserByEmail(strings.ToLower(strings.TrimSpace(req.Email)))
	default:
		writeErr(w, http.StatusBadRequest, "Укажите email пользователя")
		return
	}
	if !found || student == nil {
		writeErr(w, http.StatusNotFound, "Пользователь не найден — он должен сначала зарегистрироваться")
		return
	}
	course, ok := a.Store.GetCourseByID(courseID)
	if !ok || course == nil {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if course.Status == repo.CourseDraft {
		writeErr(w, http.StatusBadRequest, "Курс ещё не опубликован — доступ к черновику выдать нельзя")
		return
	}

	unlock, locked := a.lockBilling(w, r)
	if !locked {
		return
	}
	defer unlock()

	wasActive := false
	if e, ok := a.Store.GetEnrollment(student.ID, course.ID); ok && e.Status == repo.EnrollmentActive {
		wasActive = true
	}
	e, created := a.Store.EnsureEnrollment(student.ID, course.ID)
	if e == nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось выдать доступ к курсу")
		return
	}
	if admin, ok := a.currentUser(r); ok && !wasActive {
		log.Printf("admin %s granted course %s to user %s (created=%v)", admin.ID, course.ID, student.ID, created)
	}
	writeJSON(w, http.StatusOK, grantEnrollmentResponse{
		StudentID:   student.ID,
		Email:       student.Email,
		CourseID:    course.ID,
		CourseTitle: course.Title,
		Status:      e.Status,
		Created:     created,
		WasActive:   wasActive,
	})
}
