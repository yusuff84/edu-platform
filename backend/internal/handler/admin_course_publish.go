package handler

// T87 — принудительная публикация курса супер-админом.
//
// POST /admin/courses/{id}/publish публикует курс из draft или review без
// очереди модерации: в отличие от teacher-submit, программа и статус review
// не проверяются — админ решает сам. Ответ — конверт {"course": {...}} той же
// формы Course, что у /teacher/courses/{id}/unpublish и admin archive.
//
// Идемпотентность: повторная публикация уже published-курса — 200 без смены
// статуса (комментарий модерации очищается, если он остался). archived — 409
// до unarchive; нет курса — 404. Права — только admin: маршрут зарегистрирован
// внутри r.Route("/admin", ...) в middleware.go, adminOnly — страховка, как у
// остальных admin-хендлеров.

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// AdminPublishCourse — POST /admin/courses/{id}/publish (T87).
func (a *App) AdminPublishCourse(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	switch c.Status {
	case repo.CourseArchived:
		writeErr(w, http.StatusConflict,
			"Архивный курс нельзя опубликовать: сначала верните его из архива")
		return
	case repo.CourseDraft, repo.CourseReview:
		// Публикация без модерации: комментарий модерации очищается.
		if !a.Store.SetCourseStatus(c.ID, repo.CoursePublished, "") {
			writeErr(w, http.StatusInternalServerError, "Не удалось опубликовать курс")
			return
		}
	case repo.CoursePublished:
		// Идемпотентный повтор: статус уже целевой. Комментарий модерации
		// очищаем, только если он остался (без лишней записи и updatedAt).
		if c.ModerationComment != "" {
			a.Store.SetCourseStatus(c.ID, repo.CoursePublished, "")
		}
	default:
		writeErr(w, http.StatusConflict, "Курс в неизвестном статусе: публикация невозможна")
		return
	}
	a.respondFreshCourse(w, c.ID)
}
