package handler

import (
	"errors"
	"net/http"
	"strconv"
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

// parseRatingFilter разбирает необязательный rating (T69): пусто — фильтра
// нет (0, true), иначе только целое 1..5 (0, false — вызывающий отдаёт 400).
func parseRatingFilter(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 5 {
		return 0, false
	}
	return n, true
}

// ListReviews — GET /courses/{id}/reviews: список отзывов.
//
// T69: необязательный rating (1..5) фильтрует items/total; avgRating и
// ratingsCount всегда считаются по всем отзывам курса (фильтр на них не
// влияет). breakdown — аддитивное поле: число отзывов по каждой оценке,
// ключи-строки "1".."5" присутствуют всегда. Форма конверта прежняя.
func (a *App) ListReviews(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolveCourse(chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	rating, valid := parseRatingFilter(r.URL.Query().Get("rating"))
	if !valid {
		writeErr(w, http.StatusBadRequest, "Оценка должна быть числом от 1 до 5")
		return
	}
	list := a.Store.ListReviewsByCourse(c.ID)
	if list == nil {
		list = []*repo.Review{}
	}
	items := list
	if rating > 0 {
		items = make([]*repo.Review, 0, len(list))
		for _, rv := range list {
			if rv.Rating == rating {
				items = append(items, rv)
			}
		}
	}
	counts := a.Store.ReviewRatingBreakdown(c.ID)
	breakdown := make(map[string]int, 5)
	for n := 1; n <= 5; n++ {
		breakdown[strconv.Itoa(n)] = counts[n]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items),
		"avgRating": c.AvgRating, "ratingsCount": c.RatingsCount,
		"breakdown": breakdown,
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
