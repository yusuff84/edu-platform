package handler

// T83 — глобальная модерация отзывов: GET /admin/reviews с фильтрами по
// курсу/оценке/поиску/периоду, сортировкой и пагинацией.
//
// Маршрут регистрируется внутри r.Route("/admin", ...) в middleware.go
// (RegisterAdminReviewsRoutes): requireAuth + requireRole(admin) навешаны
// родителем, плюс adminOnly в обработчике. Существующий DELETE
// /admin/reviews/{id} остаётся в admin_console.go и здесь не дублируется.
// Коды: 400 — неверный query, 401/403 — авторизация, 200 — список по
// контракту T83 (items всегда массив, не null).

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// RegisterAdminReviewsRoutes вешает маршрут T83 на /admin-роутер.
// Путь относительный: вызывается внутри r.Route("/admin", ...).
func (a *App) RegisterAdminReviewsRoutes(r chi.Router) {
	r.Get("/reviews", a.AdminListReviews)
}

// adminGlobalReviewItem — строка глобального списка отзывов в форме контракта
// T83. body — всегда строка (отзыв без текста отдаётся как ""); ответ
// преподавателя — teacherReply/teacherReplyAt (пусто/null, если ответа нет).
type adminGlobalReviewItem struct {
	ID             string     `json:"id"`
	CourseID       string     `json:"courseId"`
	CourseTitle    string     `json:"courseTitle"`
	CourseSlug     string     `json:"courseSlug"`
	StudentID      string     `json:"studentId"`
	StudentEmail   string     `json:"studentEmail"`
	StudentName    string     `json:"studentName"`
	Rating         int        `json:"rating"`
	Body           string     `json:"body"`
	CreatedAt      time.Time  `json:"createdAt"`
	TeacherReply   string     `json:"teacherReply"`
	TeacherReplyAt *time.Time `json:"teacherReplyAt"`
}

type adminGlobalReviewListResponse struct {
	Items []adminGlobalReviewItem `json:"items"`
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Limit int                     `json:"limit"`
}

// AdminListReviews — GET /admin/reviews?courseId=&rating=&q=&from=&to=
// &sort=&dir=&page=&limit= (T83), только admin.
//
// 400 — courseId не UUID, rating вне 1..5 или не целое, from/to не
// ГГГГ-ММ-ДД или from > to, sort вне createdAt|rating, dir вне asc|desc,
// нечисловые page/limit. Поиск q — без учёта регистра по названию курса,
// email/имени ученика и тексту отзыва. total считается до пагинации.
func (a *App) AdminListReviews(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}

	q := r.URL.Query()
	courseID := strings.TrimSpace(q.Get("courseId"))
	if courseID != "" {
		if _, err := uuid.Parse(courseID); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр courseId должен быть UUID")
			return
		}
	}

	var rating *int
	if v := strings.TrimSpace(q.Get("rating")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5 {
			writeErr(w, http.StatusBadRequest, "Параметр rating должен быть целым числом от 1 до 5")
			return
		}
		rating = &n
	}

	sortParam := strings.TrimSpace(q.Get("sort"))
	switch sortParam {
	case "", "createdAt", "rating":
	default:
		writeErr(w, http.StatusBadRequest, "Параметр sort должен быть одним из: createdAt, rating")
		return
	}
	dirParam := strings.TrimSpace(q.Get("dir"))
	switch dirParam {
	case "", "asc", "desc":
	default:
		writeErr(w, http.StatusBadRequest, "Параметр dir должен быть asc или desc")
		return
	}

	// Границы периода по createdAt — тем же парсером, что журнал платежей и
	// список курсов: тексты ошибок («Дата в формате ГГГГ-ММ-ДД», «Начало
	// периода позже конца») приходят оттуда.
	from, to, ok := parseJournalPeriod(w, r)
	if !ok {
		return
	}

	rows, total := a.Store.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseID,
		Rating:   rating,
		Q:        strings.TrimSpace(q.Get("q")),
		From:     from,
		To:       to,
		Sort:     sortParam,
		Dir:      dirParam,
		Page:     page,
		Limit:    limit,
	})
	items := make([]adminGlobalReviewItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminGlobalReviewItem{
			ID: row.ID, CourseID: row.CourseID,
			CourseTitle: row.CourseTitle, CourseSlug: row.CourseSlug,
			StudentID: row.StudentID, StudentEmail: row.StudentEmail,
			StudentName: row.StudentName,
			Rating:      row.Rating, Body: row.Body, CreatedAt: row.CreatedAt,
			TeacherReply: row.TeacherReply, TeacherReplyAt: row.TeacherReplyAt,
		})
	}
	writeJSON(w, http.StatusOK, adminGlobalReviewListResponse{
		Items: items, Total: total, Page: page, Limit: limit,
	})
}
