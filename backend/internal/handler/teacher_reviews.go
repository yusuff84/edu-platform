package handler

// T69 — публичный ответ преподавателя на отзыв:
// PUT/DELETE /teacher/courses/{id}/reviews/{reviewId}/reply.
// Контракт — Task/00-api-contract.md, раздел «T69 — ответы преподавателя на
// отзывы + разбивка оценок» (заморожен).
//
// Маршруты регистрируются RegisterTeacherReviewReplyRoutes внутри
// r.Route("/teacher", ...) в middleware.go: requireAuth + requireRole(teacher,
// admin) навешаны родителем, пути относительные. Владение курсом проверяет
// существующий ownCourse (teacher.go): 404 — курса нет, 403 — чужой.
// Отзыв другого курса неотличим от несуществующего — 404 «Отзыв не найден»
// (проверку делает Store по паре courseID+reviewID).

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// reviewReplyMinRunes/maxRunes — границы текста ответа по контракту T69
// (после TrimSpace, в символах-рунах).
const (
	reviewReplyMinRunes = 3
	reviewReplyMaxRunes = 2000
)

// RegisterTeacherReviewReplyRoutes вешает маршруты T69 на /teacher-роутер
// (см. middleware.go: вызов внутри r.Route("/teacher", ...)).
func (a *App) RegisterTeacherReviewReplyRoutes(r chi.Router) {
	r.Put("/courses/{id}/reviews/{reviewId}/reply", a.TeacherSetReviewReply)
	r.Delete("/courses/{id}/reviews/{reviewId}/reply", a.TeacherDeleteReviewReply)
}

type reviewReplyRequest struct {
	Body string `json:"body"`
}

// TeacherSetReviewReply — PUT /teacher/courses/{id}/reviews/{reviewId}/reply:
// установка или замена ответа. 200 {"review": <Review>}; повторный PUT
// заменяет текст и обновляет teacherReplyAt. 400 — длина вне 3..2000 после
// trim; 403 — чужой курс; 404 — курса или отзыва (в т.ч. другого курса) нет.
func (a *App) TeacherSetReviewReply(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	var req reviewReplyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := utf8.RuneCountInString(body); n < reviewReplyMinRunes || n > reviewReplyMaxRunes {
		writeErr(w, http.StatusBadRequest, "Ответ: от 3 до 2000 символов")
		return
	}
	rev, err := a.Store.SetReviewReply(c.ID, strings.TrimSpace(chi.URLParam(r, "reviewId")), body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Отзыв не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить ответ на отзыв")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"review": rev})
}

// TeacherDeleteReviewReply — DELETE /teacher/courses/{id}/reviews/{reviewId}/reply:
// 204 без тела; отсутствие ответа — тоже 204 (идемпотентно). 403 — чужой
// курс; 404 — курса или отзыва (в т.ч. другого курса) нет.
func (a *App) TeacherDeleteReviewReply(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	err := a.Store.ClearReviewReply(c.ID, strings.TrimSpace(chi.URLParam(r, "reviewId")))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Отзыв не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить ответ на отзыв")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
