package handler

// T21 — жизненный цикл курса в конструкторе: дубликат, архивация, возврат
// из архива, отзыв с модерации, перенос урока между модулями.
//
// Маршруты регистрируются RegisterTeacherLifecycleRoutes внутри
// r.Route("/teacher", ...) в middleware.go — так же, как
// RegisterTeacherEditRoutes (T13) и RegisterTeacherCRUDRoutes (T19):
// requireAuth + requireRole(teacher, admin) уже навешаны родителем, пути
// относительные, владение курсом проверяет ownCourse (403 — чужой,
// 404 — нет курса/урока/модуля).
//
// Коды по правилу проекта: 400 — валидация тела/параметров, 409 — конфликт
// с состоянием курса, 403 — нет прав, 404 — нет сущности.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterTeacherLifecycleRoutes вешает маршруты T21 на /teacher-роутер.
func (a *App) RegisterTeacherLifecycleRoutes(r chi.Router) {
	r.Post("/courses/{id}/duplicate", a.TeacherDuplicateCourse)
	r.Post("/courses/{id}/archive", a.TeacherArchiveCourse)
	r.Post("/courses/{id}/unarchive", a.TeacherUnarchiveCourse)
	r.Post("/courses/{id}/withdraw", a.TeacherWithdrawCourse)
	r.Post("/courses/{id}/lessons/{lessonId}/move", a.TeacherMoveLesson)
}

// ---- duplicate ----

// courseCloneOwner — расширенная возможность хранилища (T21): копия курса с
// явным владельцем. Нужна, когда админ дублирует чужой курс: по контракту
// копия принадлежит вызывающему, а обязательный метод StoreAPI
// CloneCourse(courseID) идентификатора пользователя не принимает. Обе
// штатные реализации (in-memory Store и PostgresStore) её поддерживают.
type courseCloneOwner interface {
	CloneCourseAs(courseID, teacherID string) (*repo.Course, error)
}

// cloneCourseFor копирует курс так, чтобы владельцем копии стал userID:
// для своего курса хватает CloneCourse (владелец копии = владелец исходного),
// для чужого (админ) — CloneCourseAs.
func (a *App) cloneCourseFor(userID string, c *repo.Course) (*repo.Course, error) {
	if userID == c.TeacherID {
		return a.Store.CloneCourse(c.ID)
	}
	cl, ok := a.Store.(courseCloneOwner)
	if !ok {
		// Хранилище без расширения: выдать копию не тому владельцу нельзя.
		return nil, errors.New("хранилище не поддерживает копирование чужого курса")
	}
	return cl.CloneCourseAs(c.ID, userID)
}

// TeacherDuplicateCourse — POST /courses/{id}/duplicate: 201 {"course":{...}}.
// Копия — черновик вызывающего пользователя: программа копируется глубоко
// (модули и уроки с новыми id), записи/платежи/отзывы/прогресс — нет.
// 404 — нет курса; 403 — чужой курс.
func (a *App) TeacherDuplicateCourse(w http.ResponseWriter, r *http.Request) {
	u, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	dup, err := a.cloneCourseFor(u.ID, c)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Курс не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось скопировать курс")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"course": dup})
}

// ---- archive / unarchive / withdraw ----

// TeacherArchiveCourse — POST /courses/{id}/archive: published|draft|review →
// archived (200 {"course":{...}}). 409 — курс уже в архиве; 404/403 — как
// везде. Комментарий модерации сохраняется: архивация не отменяет замечаний.
func (a *App) TeacherArchiveCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if c.Status == repo.CourseArchived {
		writeErr(w, http.StatusConflict, "Курс уже в архиве")
		return
	}
	if !a.Store.SetCourseStatus(c.ID, repo.CourseArchived, c.ModerationComment) {
		writeErr(w, http.StatusInternalServerError, "Не удалось архивировать курс")
		return
	}
	a.respondFreshCourse(w, c.ID)
}

// TeacherUnarchiveCourse — POST /courses/{id}/unarchive: archived → draft
// (200 {"course":{...}}). 409 — курс не в архиве; 404/403 — как везде.
func (a *App) TeacherUnarchiveCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if c.Status != repo.CourseArchived {
		writeErr(w, http.StatusConflict, "Курс не в архиве")
		return
	}
	if !a.Store.SetCourseStatus(c.ID, repo.CourseDraft, c.ModerationComment) {
		writeErr(w, http.StatusInternalServerError, "Не удалось вернуть курс из архива")
		return
	}
	a.respondFreshCourse(w, c.ID)
}

// TeacherWithdrawCourse — POST /courses/{id}/withdraw: review → draft,
// комментарий модерации очищается (200 {"course":{...}}). 409 — курс не на
// модерации; 404/403 — как везде.
func (a *App) TeacherWithdrawCourse(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if c.Status != repo.CourseReview {
		writeErr(w, http.StatusConflict, "Курс не на модерации")
		return
	}
	if !a.Store.SetCourseStatus(c.ID, repo.CourseDraft, "") {
		writeErr(w, http.StatusInternalServerError, "Не удалось отозвать курс с модерации")
		return
	}
	a.respondFreshCourse(w, c.ID)
}

// respondFreshCourse возвращает свежий курс после смены статуса в конверте
// {"course":{...}} (T21): updatedAt/moderationComment уже пересчитаны
// хранилищем.
func (a *App) respondFreshCourse(w http.ResponseWriter, courseID string) {
	updated, _ := a.Store.GetCourseByID(courseID)
	writeJSON(w, http.StatusOK, map[string]any{"course": updated})
}

// ---- move ----

// moveLessonRequest — тело переноса урока (T21): целевой модуль.
type moveLessonRequest struct {
	ModuleID string `json:"moduleId"`
}

// TeacherMoveLesson — POST /courses/{id}/lessons/{lessonId}/move: переносит
// урок в конец другого модуля того же курса, 200 {"lesson":{...}}.
// 400 — пустой moduleId («Не указан модуль») или модуль не из этого курса;
// 404 — урока нет в этом курсе; 409 — курс не черновик; повторный перенос
// в тот же модуль — идемпотентный 200 без изменений.
func (a *App) TeacherMoveLesson(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	var req moveLessonRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	moduleID := strings.TrimSpace(req.ModuleID)
	if moduleID == "" {
		writeErr(w, http.StatusBadRequest, "Не указан модуль")
		return
	}
	mod, found := a.Store.GetModule(moduleID)
	if !found || mod.CourseID != c.ID {
		writeErr(w, http.StatusBadRequest, "Модуль не принадлежит этому курсу")
		return
	}
	l, found := a.Store.GetLesson(chi.URLParam(r, "lessonId"))
	if !found || !a.lessonInCourse(l, c.ID) {
		writeErr(w, http.StatusNotFound, "Урок не найден")
		return
	}
	if c.Status != repo.CourseDraft {
		writeErr(w, http.StatusConflict, "Менять структуру можно только в черновике")
		return
	}
	if l.ModuleID != mod.ID {
		if err := a.Store.MoveLessonToModule(l.ID, mod.ID); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "Урок не найден")
				return
			}
			writeErr(w, http.StatusInternalServerError, "Не удалось перенести урок")
			return
		}
		a.Store.TouchCourse(c.ID)
	}
	updated, _ := a.Store.GetLesson(l.ID)
	writeJSON(w, http.StatusOK, map[string]any{"lesson": updated})
}
