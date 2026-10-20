package handler

// T71 — вопросы и ответы по урокам (волна P9).
// Контракт — Task/00-api-contract.md, раздел «T71 — Вопросы и ответы по
// урокам» (заморожен):
//   - GET  /courses/{id}/questions?lessonId=&page=&limit= — публичный;
//   - POST /courses/{id}/questions — записанные, владелец курса, admin;
//   - PATCH /courses/{id}/questions/{questionId} — автор вопроса;
//   - POST /courses/{id}/questions/{questionId}/answers — те же права;
//   - PATCH /courses/{id}/questions/{questionId}/answers/{answerId} — автор
//     ответа;
//   - DELETE /courses/{id}/questions/{questionId} — автор, владелец, admin;
//   - DELETE /courses/{id}/questions/{questionId}/answers/{answerId} — автор
//     ответа, владелец, admin.
//
// Все ошибки и тексты — строго по контракту. Проверки прав переиспользуют
// существующие хелперы: currentUser (middleware.go), активную запись через
// Store.GetEnrollment, владение курсом — сравнение Course.TeacherID (ownCourse
// не подходит: он отвечает 403 «Это чужой курс» и требует role teacher/admin,
// а здесь владельцу-админу нужны разные ответы 404/403 по контракту).

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

const (
	questionMinRunes = 10
	questionMaxRunes = 2000
	answerMinRunes   = 3
	answerMaxRunes   = 2000
)

// courseByIDForQA достаёт курс по UUID из пути (контракт T71: только id, не
// slug). false — handler уже ответил 404 «Курс не найден».
func (a *App) courseByIDForQA(w http.ResponseWriter, r *http.Request) (*repo.Course, bool) {
	c, ok := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return nil, false
	}
	return c, true
}

// courseVisibleForQA — публичная видимость курса в ленте вопросов: published
// виден всем, остальные статусы — только владельцу и админу; прочим 404
// «Курс не найден» (контракт T71). u == nil — гость.
func courseVisibleForQA(c *repo.Course, u *repo.User) bool {
	if c.Status == repo.CoursePublished {
		return true
	}
	return u != nil && (u.Role == repo.RoleAdmin || c.TeacherID == u.ID)
}

// courseHasLesson — принадлежит ли урок курсу (через модуль). Для фильтра
// lessonId и проверки «урок не из курса» (404 «Урок не найден в этом курсе»).
func (a *App) courseHasLesson(courseID, lessonID string) bool {
	for _, l := range a.Store.ListLessonsByCourse(courseID) {
		if l.ID == lessonID {
			return true
		}
	}
	return false
}

// canParticipateInQA — активная запись на курс, владелец курса или admin
// (контракт T71: писать вопросы и ответы могут только они).
func (a *App) canParticipateInQA(c *repo.Course, u *repo.User) bool {
	if u.Role == repo.RoleAdmin || c.TeacherID == u.ID {
		return true
	}
	e, ok := a.Store.GetEnrollment(u.ID, c.ID)
	return ok && e.Status == repo.EnrollmentActive
}

// canModerateQA — автор сообщения, владелец курса или admin (право удаления).
func canModerateQA(c *repo.Course, u *repo.User, authorID string) bool {
	return u.ID == authorID || u.Role == repo.RoleAdmin || c.TeacherID == u.ID
}

// parseQAPositiveInt разбирает page/limit ленты вопросов. Нечисловое,
// пустое или вне диапазона значение — дефолт (контракт T71: «нечисловые/вне
// диапазона → дефолт»); max <= 0 — верхней границы нет.
func parseQAPositiveInt(raw string, def, max int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || (max > 0 && n > max) {
		return def
	}
	return n
}

// ListLessonQuestions — GET /courses/{id}/questions: публичная лента вопросов
// курса. items — новые первыми, ответы внутри — старые первыми; total — до
// пагинации. Неопубликованный курс для не-владельца/не-админа — 404.
func (a *App) ListLessonQuestions(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	if !courseVisibleForQA(c, a.optionalUser(r)) {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	q := r.URL.Query()
	lessonID := strings.TrimSpace(q.Get("lessonId"))
	if lessonID != "" && !a.courseHasLesson(c.ID, lessonID) {
		writeErr(w, http.StatusNotFound, "Урок не найден в этом курсе")
		return
	}
	page := parseQAPositiveInt(q.Get("page"), 1, 0)
	limit := parseQAPositiveInt(q.Get("limit"), 20, 100)
	items, total := a.Store.ListLessonQuestions(c.ID, lessonID, page, limit)
	if items == nil {
		items = []*repo.LessonQuestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
	})
}

type createLessonQuestionRequest struct {
	LessonID string `json:"lessonId"`
	Body     string `json:"body"`
}

// CreateLessonQuestion — POST /courses/{id}/questions: только записанные на
// курс, владелец курса и admin. 201 {"question": LessonQuestion} с пустым
// answers.
func (a *App) CreateLessonQuestion(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	if !a.canParticipateInQA(c, u) {
		writeErr(w, http.StatusForbidden, "Задавать вопросы могут только записанные на курс")
		return
	}
	var req createLessonQuestionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := utf8.RuneCountInString(body); n < questionMinRunes || n > questionMaxRunes {
		writeErr(w, http.StatusBadRequest, "Вопрос: от 10 до 2000 символов")
		return
	}
	lessonID := strings.TrimSpace(req.LessonID)
	if lessonID == "" || !a.courseHasLesson(c.ID, lessonID) {
		writeErr(w, http.StatusNotFound, "Урок не найден в этом курсе")
		return
	}
	q, err := a.Store.CreateLessonQuestion(c.ID, lessonID, u.ID, body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Урок не найден в этом курсе")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить вопрос")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"question": q})
}

type createLessonAnswerRequest struct {
	Body string `json:"body"`
}

// CreateLessonAnswer — POST /courses/{id}/questions/{questionId}/answers.
// 201 {"answer": LessonAnswer}. Вопрос другого курса неотличим от
// несуществующего — 404 «Вопрос не найден» (контракт T71).
func (a *App) CreateLessonAnswer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	questionID := strings.TrimSpace(chi.URLParam(r, "questionId"))
	if _, found := a.Store.GetLessonQuestion(c.ID, questionID); !found {
		writeErr(w, http.StatusNotFound, "Вопрос не найден")
		return
	}
	if !a.canParticipateInQA(c, u) {
		writeErr(w, http.StatusForbidden, "Отвечать могут только записанные на курс")
		return
	}
	var req createLessonAnswerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := utf8.RuneCountInString(body); n < answerMinRunes || n > answerMaxRunes {
		writeErr(w, http.StatusBadRequest, "Ответ: от 3 до 2000 символов")
		return
	}
	ans, err := a.Store.CreateLessonAnswer(c.ID, questionID, u.ID, body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Вопрос не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить ответ")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"answer": ans})
}

type updateLessonTextRequest struct {
	Body string `json:"body"`
}

// UpdateLessonQuestion — PATCH /courses/{id}/questions/{questionId}: правка
// текста вопроса его автором. 200 {"question": LessonQuestion}. Владелец
// курса и admin могут удалить вопрос, но не переписать чужой текст — 403.
func (a *App) UpdateLessonQuestion(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	questionID := strings.TrimSpace(chi.URLParam(r, "questionId"))
	q, found := a.Store.GetLessonQuestion(c.ID, questionID)
	if !found {
		writeErr(w, http.StatusNotFound, "Вопрос не найден")
		return
	}
	if q.StudentID != u.ID {
		writeErr(w, http.StatusForbidden, "Нет прав на редактирование")
		return
	}
	var req updateLessonTextRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := utf8.RuneCountInString(body); n < questionMinRunes || n > questionMaxRunes {
		writeErr(w, http.StatusBadRequest, "Вопрос: от 10 до 2000 символов")
		return
	}
	updated, err := a.Store.UpdateLessonQuestionBody(c.ID, questionID, body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Вопрос не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить вопрос")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"question": updated})
}

// UpdateLessonAnswer — PATCH
// /courses/{id}/questions/{questionId}/answers/{answerId}: правка текста
// ответа его автором. 200 {"answer": LessonAnswer}. Владелец курса и admin
// могут удалить ответ, но не переписать чужой текст — 403.
func (a *App) UpdateLessonAnswer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	questionID := strings.TrimSpace(chi.URLParam(r, "questionId"))
	answerID := strings.TrimSpace(chi.URLParam(r, "answerId"))
	ans, found := a.Store.GetLessonAnswer(c.ID, questionID, answerID)
	if !found {
		writeErr(w, http.StatusNotFound, "Ответ не найден")
		return
	}
	if ans.AuthorID != u.ID {
		writeErr(w, http.StatusForbidden, "Нет прав на редактирование")
		return
	}
	var req updateLessonTextRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := utf8.RuneCountInString(body); n < answerMinRunes || n > answerMaxRunes {
		writeErr(w, http.StatusBadRequest, "Ответ: от 3 до 2000 символов")
		return
	}
	updated, err := a.Store.UpdateLessonAnswerBody(c.ID, questionID, answerID, body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Ответ не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить ответ")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": updated})
}

// DeleteLessonQuestion — DELETE /courses/{id}/questions/{questionId}: автор
// вопроса, владелец курса или admin; ответы удаляются каскадом. 204.
func (a *App) DeleteLessonQuestion(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	questionID := strings.TrimSpace(chi.URLParam(r, "questionId"))
	q, found := a.Store.GetLessonQuestion(c.ID, questionID)
	if !found {
		writeErr(w, http.StatusNotFound, "Вопрос не найден")
		return
	}
	if !canModerateQA(c, u, q.StudentID) {
		writeErr(w, http.StatusForbidden, "Нет прав на удаление")
		return
	}
	if err := a.Store.DeleteLessonQuestion(c.ID, questionID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Вопрос не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить вопрос")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteLessonAnswer — DELETE
// /courses/{id}/questions/{questionId}/answers/{answerId}: автор ответа,
// владелец курса или admin. 204.
func (a *App) DeleteLessonAnswer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, ok := a.courseByIDForQA(w, r)
	if !ok {
		return
	}
	questionID := strings.TrimSpace(chi.URLParam(r, "questionId"))
	answerID := strings.TrimSpace(chi.URLParam(r, "answerId"))
	ans, found := a.Store.GetLessonAnswer(c.ID, questionID, answerID)
	if !found {
		writeErr(w, http.StatusNotFound, "Ответ не найден")
		return
	}
	if !canModerateQA(c, u, ans.AuthorID) {
		writeErr(w, http.StatusForbidden, "Нет прав на удаление")
		return
	}
	if err := a.Store.DeleteLessonAnswer(c.ID, questionID, answerID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Ответ не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить ответ")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
