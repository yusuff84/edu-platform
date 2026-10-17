package handler

// T23 — эндпоинты кабинета ученика (потребитель — T30):
//   GET   /me/courses/{id}/progress — прогресс по курсу списком пройденных уроков;
//   GET   /me/profile              — профиль текущего пользователя;
//   PATCH /me/profile              — смена отображаемого имени;
//   GET   /me/payments             — история платежей ученика.
//
// Раньше прогресс считался эвристикой на клиенте (MyCourses.tsx сравнивал
// счётчики курса с локальным состоянием), из-за чего врал после перезахода.
// Теперь источник правды — сервер: completedLessonIds всегда массив
// (пустой — []), а не null.

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterMeExtraRoutes вешает эндпоинты T23 на /api/v1: пути абсолютные
// (/me/*), requireAuth — здесь же через .With (монтирование на уровне
// /api/v1, рядом с уже существующими /me/courses, см. middleware.go).
func (a *App) RegisterMeExtraRoutes(r chi.Router) {
	r.With(a.requireAuth).Get("/me/courses/{id}/progress", a.MyCourseProgress)
	r.With(a.requireAuth).Get("/me/profile", a.GetMyProfile)
	r.With(a.requireAuth).Patch("/me/profile", a.UpdateMyProfile)
	r.With(a.requireAuth).Get("/me/payments", a.MyPayments)
}

// ---- GET /me/courses/{id}/progress ----

// courseProgressResponse — прогресс ученика по курсу (контракт T30).
// NextLessonID — указатель: null, когда пройдено всё; CompletedLessonIDs —
// всегда не nil, иначе json.Marshal отдал бы null вместо [].
type courseProgressResponse struct {
	CourseID           string   `json:"courseId"`
	CompletedLessonIDs []string `json:"completedLessonIds"`
	CompletedLessons   int      `json:"completedLessons"`
	TotalLessons       int      `json:"totalLessons"`
	Percent            int      `json:"percent"`
	NextLessonID       *string  `json:"nextLessonId"`
	Completed          bool     `json:"completed"`
}

// MyCourseProgress — GET /me/courses/{id}/progress: список пройденных уроков
// в порядке программы, счётчики, процент и следующий непройденный урок.
// 404 — курса нет; 403 — нет активной записи (refunded/revoked доступа не дают).
func (a *App) MyCourseProgress(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	courseID := chi.URLParam(r, "id")
	c, found := a.Store.GetCourseByID(courseID)
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	e, exists := a.Store.GetEnrollment(u.ID, c.ID)
	if !exists || e.Status != repo.EnrollmentActive {
		writeErr(w, http.StatusForbidden, "Сначала купите курс, чтобы видеть прогресс")
		return
	}

	completedIDs := a.Store.ListCompletedLessonIDs(u.ID, c.ID)
	if completedIDs == nil {
		// Страховка от nil в хранилище: контракт запрещает null в JSON.
		completedIDs = []string{}
	}
	passed := make(map[string]bool, len(completedIDs))
	for _, id := range completedIDs {
		passed[id] = true
	}
	// nextLessonId — первый непройденный урок по порядку программы
	// (модуль за модулем, позиция внутри модуля).
	var next *string
	for _, l := range a.Store.ListLessonsByCourse(c.ID) {
		if !passed[l.ID] {
			id := l.ID
			next = &id
			break
		}
	}

	total := a.Store.CountLessonsByCourse(c.ID)
	done := len(completedIDs)
	percent := 0
	if total > 0 {
		percent = done * 100 / total
	}
	writeJSON(w, http.StatusOK, courseProgressResponse{
		CourseID:           c.ID,
		CompletedLessonIDs: completedIDs,
		CompletedLessons:   done,
		TotalLessons:       total,
		Percent:            percent,
		NextLessonID:       next,
		Completed:          total > 0 && done == total,
	})
}

// ---- GET/PATCH /me/profile ----

// profileResponse — профиль ученика (контракт T30): displayName — всегда
// строка (пустая, если имя не задано), createdAt — RFC3339.
type profileResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Role        repo.Role `json:"role"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

func profileResponseOf(u *repo.User) profileResponse {
	return profileResponse{
		ID: u.ID, Email: u.Email, Role: u.Role,
		DisplayName: u.DisplayName, CreatedAt: u.CreatedAt,
	}
}

// GetMyProfile — GET /me/profile: профиль текущего пользователя.
func (a *App) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	writeJSON(w, http.StatusOK, profileResponseOf(u))
}

type updateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

// UpdateMyProfile — PATCH /me/profile: смена отображаемого имени.
// Лишние поля отклоняет decodeJSON (DisallowUnknownFields); имя тримится,
// длина 2..60 символов (в рунах — русские имена считаются по символам).
func (a *App) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	var req updateProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if n := utf8.RuneCountInString(name); n < 2 || n > 60 {
		writeErr(w, http.StatusBadRequest, "Имя должно содержать от 2 до 60 символов")
		return
	}
	if !a.Store.SetUserDisplayName(u.ID, name) {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить имя")
		return
	}
	// Перечитываем пользователя: в Postgres-режиме u — снимок до UPDATE.
	fresh, ok := a.Store.GetUserByID(u.ID)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить имя")
		return
	}
	writeJSON(w, http.StatusOK, profileResponseOf(fresh))
}

// ---- GET /me/payments ----

// myPaymentItem — строка истории платежей ученика (контракт T30):
// courseTitle подтягивается из курса (пустая строка, если курс удалён),
// processedAt — null, пока платёж не обработан.
type myPaymentItem struct {
	ID          string             `json:"id"`
	CourseID    string             `json:"courseId"`
	CourseTitle string             `json:"courseTitle"`
	AmountCents int                `json:"amountCents"`
	Currency    string             `json:"currency"`
	Status      repo.PaymentStatus `json:"status"`
	Provider    string             `json:"provider"`
	CreatedAt   time.Time          `json:"createdAt"`
	ProcessedAt *time.Time         `json:"processedAt"`
}

type myPaymentsResponse struct {
	Items []myPaymentItem `json:"items"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// MyPayments — GET /me/payments?page=&limit=: платежи ученика во всех
// статусах, новые первыми (ученик должен видеть и failed). Пагинация —
// существующим paginate (страница 1-based, limit по умолчанию 20, максимум 100).
func (a *App) MyPayments(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	all := a.Store.ListPaymentsByStudent(u.ID)
	start, safePage, safeLimit := paginate(len(all), page, limit)
	end := start + safeLimit
	if end > len(all) {
		end = len(all)
	}
	items := make([]myPaymentItem, 0, end-start)
	for _, p := range all[start:end] {
		title := ""
		if c, found := a.Store.GetCourseByID(p.CourseID); found {
			title = c.Title
		}
		items = append(items, myPaymentItem{
			ID: p.ID, CourseID: p.CourseID, CourseTitle: title,
			AmountCents: p.AmountCents, Currency: p.Currency,
			Status: p.Status, Provider: p.Provider,
			CreatedAt: p.CreatedAt, ProcessedAt: p.ProcessedAt,
		})
	}
	writeJSON(w, http.StatusOK, myPaymentsResponse{
		Items: items, Total: len(all), Page: safePage, Limit: safeLimit,
	})
}
