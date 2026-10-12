package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// resolveCourse ищет курс по id, а затем по slug (для /courses/{id}/reviews).
func (a *App) resolveCourse(idOrSlug string) (*repo.Course, bool) {
	if c, ok := a.Store.GetCourseByID(strings.TrimSpace(idOrSlug)); ok {
		return c, true
	}
	return a.Store.GetCourseBySlug(idOrSlug)
}

// ListReviews — GET /courses/{id}/reviews: список отзывов.
func (a *App) ListReviews(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolveCourse(chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	list := a.Store.ListReviewsByCourse(c.ID)
	if list == nil {
		list = []*repo.Review{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": list, "total": len(list),
		"avgRating": c.AvgRating, "ratingsCount": c.RatingsCount,
	})
}

type createReviewRequest struct {
	Rating int    `json:"rating"`
	Body   string `json:"body"`
}

// CreateReview — POST /courses/{id}/reviews: только записанные, один отзыв.
func (a *App) CreateReview(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	c, found := a.resolveCourse(chi.URLParam(r, "id"))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	e, exists := a.Store.GetEnrollment(u.ID, c.ID)
	if !exists || e.Status != repo.EnrollmentActive {
		writeErr(w, http.StatusForbidden, "Оставить отзыв могут только ученики этого курса")
		return
	}
	var req createReviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		writeErr(w, http.StatusBadRequest, "Оценка должна быть от 1 до 5")
		return
	}
	rev, err := a.Store.CreateReview(u.ID, c.ID, req.Rating, strings.TrimSpace(req.Body))
	if err != nil {
		if errors.Is(err, repo.ErrAlreadyExists) {
			writeErr(w, http.StatusConflict, "Вы уже оставили отзыв на этот курс")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить отзыв")
		return
	}
	a.Store.RecalcCourseRating(c.ID)
	writeJSON(w, http.StatusCreated, rev)
}
