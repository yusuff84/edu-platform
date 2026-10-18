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

// adminReviewQueueItem — элемент очереди модерации: полная форма Course
// (встраивание указателя разворачивает все JSON-поля, как teacherCourseItem
// в teacher_crud.go) + T48: email автора и объём программы. Прежние поля
// контракта T07/T25 не меняются.
type adminReviewQueueItem struct {
	*repo.Course
	TeacherEmail string `json:"teacherEmail"`
	ModulesCount int    `json:"modulesCount"`
	LessonsCount int    `json:"lessonsCount"`
}

// AdminReviewQueue — GET /admin/courses/review: очередь модерации.
// T48: к каждому элементу аддитивно добавлены teacherEmail ("" — автор не
// найден), modulesCount и lessonsCount — одним вызовом CourseContentCounts,
// без N+1 (как в списке курсов админки T25); email — с кэшем по id автора.
func (a *App) AdminReviewQueue(w http.ResponseWriter, r *http.Request) {
	list := a.Store.ListCoursesByStatus(repo.CourseReview)
	counts := a.Store.CourseContentCounts()
	emails := make(map[string]string)
	items := make([]adminReviewQueueItem, 0, len(list))
	for _, c := range list {
		email, cached := emails[c.TeacherID]
		if !cached {
			if u, ok := a.Store.GetUserByID(c.TeacherID); ok {
				email = u.Email
			}
			emails[c.TeacherID] = email
		}
		cc := counts[c.ID]
		items = append(items, adminReviewQueueItem{
			Course: c, TeacherEmail: email,
			ModulesCount: cc.Modules, LessonsCount: cc.Lessons,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
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
