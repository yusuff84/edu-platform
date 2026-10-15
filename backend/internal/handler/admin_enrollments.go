package handler

import (
	"errors"
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
