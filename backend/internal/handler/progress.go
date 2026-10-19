package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
	"edu-platform/backend/internal/service"
)

// courseProgress считает прогресс ученика по курсу и следующий урок.
func (a *App) courseProgress(studentID string, c *repo.Course) (percent int, next map[string]any) {
	lessons := a.Store.ListLessonsByCourse(c.ID)
	if len(lessons) == 0 {
		return 0, nil
	}
	done := 0
	for _, l := range lessons {
		if a.Store.IsLessonCompleted(studentID, l.ID) {
			done++
		} else if next == nil {
			next = map[string]any{"lessonId": l.ID, "title": l.Title, "kind": l.Kind}
		}
	}
	return done * 100 / len(lessons), next
}

// MyCourses — GET /me/courses: курсы ученика с прогрессом.
func (a *App) MyCourses(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	enrolls := a.Store.ListEnrollmentsByStudent(u.ID)
	// T67: номер сертификата в элементе /me/courses (пустая строка — нет).
	certificates := make(map[string]string)
	for _, cert := range a.Store.ListCertificatesByStudent(u.ID) {
		certificates[cert.CourseID] = cert.Number
	}
	items := make([]map[string]any, 0, len(enrolls))
	for _, e := range enrolls {
		if e.Status != repo.EnrollmentActive {
			continue
		}
		c, found := a.Store.GetCourseByID(e.CourseID)
		if !found {
			continue
		}
		percent, next := a.courseProgress(u.ID, c)
		items = append(items, map[string]any{
			"course": toCourseItem(c), "percent": percent, "nextLesson": next,
			"certificateNumber": certificates[c.ID],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type completeRequest struct {
	QuizAnswers map[string]int `json:"quizAnswers"`
	Code        string         `json:"code"`
	Language    string         `json:"language"`
}

// CompleteLesson — POST /me/courses/{id}/lessons/{lessonId}/complete.
// video/live — сразу; quiz — проверка ответов; code — прокси в runner (таймаут 5с).
func (a *App) CompleteLesson(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	courseID := chi.URLParam(r, "id")
	lessonID := chi.URLParam(r, "lessonId")
	c, found := a.Store.GetCourseByID(courseID)
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	l, found := a.Store.GetLesson(lessonID)
	if !found {
		writeErr(w, http.StatusNotFound, "Урок не найден")
		return
	}
	// Урок должен принадлежать курсу.
	belongs := false
	for _, cl := range a.Store.ListLessonsByCourse(c.ID) {
		if cl.ID == l.ID {
			belongs = true
			break
		}
	}
	if !belongs {
		writeErr(w, http.StatusBadRequest, "Урок не принадлежит этому курсу")
		return
	}
	e, exists := a.Store.GetEnrollment(u.ID, c.ID)
	if !exists || e.Status != repo.EnrollmentActive {
		writeErr(w, http.StatusForbidden, "Сначала купите курс, чтобы проходить уроки")
		return
	}

	var req completeRequest
	if r.ContentLength != 0 {
		// Тело опционально (для video/live его может не быть).
		if !decodeJSON(w, r, &req) {
			return
		}
	}

	finish := func() {
		a.Store.CompleteLesson(u.ID, l.ID)
		// T67: если этим завершением все уроки курса пройдены — выдаём
		// сертификат (идемпотентно, повторный сертификат не создаётся).
		a.issueCertificateIfCourseCompleted(u.ID, c)
		percent, next := a.courseProgress(u.ID, c)
		writeJSON(w, http.StatusOK, map[string]any{
			"passed": true, "completed": true, "percent": percent, "nextLesson": next,
		})
	}

	// failWithProgress отвечает 200 при НЕУСПЕШНОЙ попытке. T16 (п.2): кроме
	// message возвращаем текущие percent и nextLesson — по той же логике, что
	// и при успехе, чтобы фронт обновлял ленту прогресса без рефетча /me/courses.
	failWithProgress := func(message string, extra map[string]any) {
		percent, next := a.courseProgress(u.ID, c)
		payload := map[string]any{
			"passed": false, "completed": false,
			"message": message, "percent": percent, "nextLesson": next,
		}
		for k, v := range extra {
			payload[k] = v
		}
		writeJSON(w, http.StatusOK, payload)
	}

	switch l.Kind {
	case repo.LessonVideoText, repo.LessonLive:
		finish()
	case repo.LessonQuiz:
		if l.Quiz == nil {
			writeErr(w, http.StatusInternalServerError, "Квиз урока не настроен")
			return
		}
		if len(req.QuizAnswers) == 0 {
			writeErr(w, http.StatusBadRequest, "Пришлите ответы на вопросы (quizAnswers)")
			return
		}
		passed, correct, total := service.CheckQuiz(l.Quiz, req.QuizAnswers)
		if !passed {
			a.Store.BumpAttempt(u.ID, l.ID)
			failWithProgress("Не все ответы верны, попробуйте ещё раз", map[string]any{
				"correct": correct, "total": total,
			})
			return
		}
		finish()
	case repo.LessonCode:
		if l.CodeTask == nil {
			writeErr(w, http.StatusInternalServerError, "Задание урока не настроено")
			return
		}
		if strings.TrimSpace(req.Code) == "" {
			writeErr(w, http.StatusBadRequest, "Пришлите код решения (code)")
			return
		}
		lang := strings.TrimSpace(req.Language)
		if lang == "" {
			lang = l.CodeTask.Language
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		passed, message, err := service.CheckCode(ctx, a.Config.RunnerURL, lang, req.Code, l.CodeTask.Tests)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if !passed {
			a.Store.BumpAttempt(u.ID, l.ID)
			failWithProgress(message, nil)
			return
		}
		finish()
	default:
		writeErr(w, http.StatusBadRequest, "Неизвестный тип урока")
	}
}
