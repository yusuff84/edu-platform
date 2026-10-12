package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/repo"
)

type createInviteRequest struct {
	Email string `json:"email"`
}

// AdminCreateInvite — POST /admin/invites.
// Возвращает токен в ответе (MVP; в проде отправлять письмом). Старые активные
// приглашения на тот же email отзываются.
func (a *App) AdminCreateInvite(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	var req createInviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || len(email) < 5 {
		writeErr(w, http.StatusBadRequest, "Некорректный email")
		return
	}
	token, tokenHash, prefix, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось создать приглашение")
		return
	}
	inv := a.Store.CreateInvite(email, tokenHash, prefix, u.ID, time.Now().UTC().Add(auth.InviteTTL))
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": inv.ID, "email": inv.Email, "token": token,
		"tokenPrefix": inv.TokenPrefix, "expiresAt": inv.ExpiresAt,
	})
}

// AdminReviewQueue — GET /admin/courses/review: очередь модерации.
func (a *App) AdminReviewQueue(w http.ResponseWriter, r *http.Request) {
	list := a.Store.ListCoursesByStatus(repo.CourseReview)
	if list == nil {
		list = []*repo.Course{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list)})
}

type moderateRequest struct {
	Decision string `json:"decision"` // approve | reject
	Comment  string `json:"comment"`
}

// AdminModerateCourse — POST /admin/courses/{id}/moderate.
func (a *App) AdminModerateCourse(w http.ResponseWriter, r *http.Request) {
	var req moderateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	c, found := a.Store.GetCourseByID(chi.URLParam(r, "id"))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if c.Status != repo.CourseReview {
		writeErr(w, http.StatusBadRequest, "Курс не находится на модерации")
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Decision)) {
	case "approve":
		a.Store.SetCourseStatus(c.ID, repo.CoursePublished, strings.TrimSpace(req.Comment))
	case "reject":
		if strings.TrimSpace(req.Comment) == "" {
			writeErr(w, http.StatusBadRequest, "При отклонении нужен комментарий для учителя")
			return
		}
		a.Store.SetCourseStatus(c.ID, repo.CourseDraft, strings.TrimSpace(req.Comment))
	default:
		writeErr(w, http.StatusBadRequest, "Решение: approve или reject")
		return
	}
	updated, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, updated)
}
