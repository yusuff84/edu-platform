package handler

// T22 — ученики курса и статистика для учителя (таб «Ученики» конструктора;
// потребитель — фронтенд T29).
//
// Маршруты регистрируются RegisterTeacherStudentsRoutes внутри
// r.Route("/teacher", ...) в middleware.go: requireAuth + requireRole(teacher,
// admin) навешаны родителем, пути относительные, владение курсом проверяет
// существующий ownCourse (teacher.go) — 404 для несуществующего курса,
// 403 для чужого.
//
// JSON-поля отдаются ровно в форме Task/00-api-contract.md (T22): фронтенд
// пишется параллельно и ждёт studentId, email, displayName, enrolledAt,
// status, completedLessons, totalLessons, percent, lastActivityAt; items —
// всегда массив, lastActivityAt — null, если ученик не начинал курс.

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterTeacherStudentsRoutes вешает маршруты T22 на /teacher-роутер
// (см. middleware.go: вызов внутри r.Route("/teacher", ...)).
func (a *App) RegisterTeacherStudentsRoutes(r chi.Router) {
	r.Get("/courses/{id}/students", a.TeacherCourseStudents)
	r.Get("/courses/{id}/stats", a.TeacherCourseStats)
}

// studentListItem — строка списка учеников курса (T22). Имена полей — ровно
// по контракту; lastActivityAt — указатель, чтобы не начинавший ученик
// получил null, а не нулевое время.
type studentListItem struct {
	StudentID        string                `json:"studentId"`
	Email            string                `json:"email"`
	DisplayName      string                `json:"displayName"`
	EnrolledAt       time.Time             `json:"enrolledAt"`
	Status           repo.EnrollmentStatus `json:"status"`
	CompletedLessons int                   `json:"completedLessons"`
	TotalLessons     int                   `json:"totalLessons"`
	Percent          int                   `json:"percent"`
	LastActivityAt   *time.Time            `json:"lastActivityAt"`
}

// studentsResponse — конверт списка учеников: items всегда массив.
type studentsResponse struct {
	Items []studentListItem `json:"items"`
	Total int               `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}

// studentPercent — целое 0..100: completed * 100 / totalLessons
// (целочисленное деление — округление вниз); при totalLessons == 0 → 0.
// Клампинг сверху — страховка от рассинхрона данных (урок удалён из БД
// мимо каскада): контракт гарантирует 0..100.
func studentPercent(completed, totalLessons int) int {
	if totalLessons <= 0 {
		return 0
	}
	p := completed * 100 / totalLessons
	if p > 100 {
		p = 100
	}
	return p
}

// TeacherCourseStudents — GET /teacher/courses/{id}/students:
// ученики курса с прогрессом, новые записи первыми; q — поиск по email или
// displayName (подстрока, регистронезависимо, TrimSpace); page/limit —
// пагинация (limit по умолчанию 20, максимум 100). Включены записи во всех
// статусах. 400 — нечисловые page/limit, 404 — курса нет, 403 — чужой.
func (a *App) TeacherCourseStudents(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	enrollments := a.Store.ListEnrollmentsByCourse(c.ID)
	progress := a.Store.StudentProgress(c.ID)
	totalLessons := a.Store.CountLessonsByCourse(c.ID)

	// Email и displayName в StoreAPI добираются по одной записи (GetUserByID):
	// список записей и агрегат прогресса их не содержат. Без поиска (q == "")
	// вызовы делаются только для усечённой пагинацией страницы — не более 100
	// (limit ≤ 100), это допустимо. При q != "" иначе нельзя: отфильтровать по
	// email/имени можно, лишь получив их, поэтому вызывается столько раз,
	// сколько записей на курсе; повторные вызовы исключает кэш.
	users := make(map[string]*repo.User)
	userOf := func(id string) *repo.User {
		if u, cached := users[id]; cached {
			return u
		}
		u, _ := a.Store.GetUserByID(id)
		users[id] = u
		return u
	}

	if q != "" {
		filtered := make([]*repo.Enrollment, 0, len(enrollments))
		for _, e := range enrollments {
			u := userOf(e.StudentID)
			if u == nil {
				continue
			}
			if strings.Contains(strings.ToLower(u.Email), q) ||
				strings.Contains(strings.ToLower(u.DisplayName), q) {
				filtered = append(filtered, e)
			}
		}
		enrollments = filtered
	}

	total := len(enrollments)
	start, safePage, safeLimit := paginate(total, page, limit)
	end := start + safeLimit
	if end > total {
		end = total
	}
	items := make([]studentListItem, 0, end-start)
	for _, e := range enrollments[start:end] {
		sp := progress[e.StudentID]
		item := studentListItem{
			StudentID:        e.StudentID,
			EnrolledAt:       e.CreatedAt,
			Status:           e.Status,
			CompletedLessons: sp.Completed,
			TotalLessons:     totalLessons,
			Percent:          studentPercent(sp.Completed, totalLessons),
		}
		if u := userOf(e.StudentID); u != nil {
			item.Email = u.Email
			item.DisplayName = u.DisplayName
		}
		if !sp.LastActivity.IsZero() {
			last := sp.LastActivity
			item.LastActivityAt = &last
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, studentsResponse{
		Items: items, Total: total, Page: safePage, Limit: safeLimit,
	})
}

// courseStatsResponse — ответ GET /teacher/courses/{id}/stats (T22).
type courseStatsResponse struct {
	StudentsTotal     int     `json:"studentsTotal"`
	StudentsActive    int     `json:"studentsActive"`
	StudentsCompleted int     `json:"studentsCompleted"`
	AvgPercent        int     `json:"avgPercent"`
	RevenueCents      int     `json:"revenueCents"`
	ModulesTotal      int     `json:"modulesTotal"`
	LessonsTotal      int     `json:"lessonsTotal"`
	ReviewsCount      int     `json:"reviewsCount"`
	AvgRating         float64 `json:"avgRating"`
}

// TeacherCourseStats — GET /teacher/courses/{id}/stats: сводка по курсу.
// studentsTotal — все записи, studentsCompleted — ученики с percent == 100
// (при lessonsTotal > 0), avgPercent — среднее по записям с округлением вниз,
// revenueCents — сумма amountCents только успешных платежей (без вычетов),
// отзывы — из ListReviewsByCourse. 404 — курса нет, 403 — чужой.
func (a *App) TeacherCourseStats(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	enrollments := a.Store.ListEnrollmentsByCourse(c.ID)
	progress := a.Store.StudentProgress(c.ID)
	totalLessons := a.Store.CountLessonsByCourse(c.ID)

	stats := courseStatsResponse{
		StudentsTotal: len(enrollments),
		ModulesTotal:  len(a.Store.ListModulesByCourse(c.ID)),
		LessonsTotal:  totalLessons,
	}
	// avgPercent: проценты считаются целочисленно, сумма — в int, поэтому
	// sumPercent / studentsTotal — среднее с округлением вниз; без записей 0.
	sumPercent := 0
	for _, e := range enrollments {
		if e.Status == repo.EnrollmentActive {
			stats.StudentsActive++
		}
		pct := studentPercent(progress[e.StudentID].Completed, totalLessons)
		sumPercent += pct
		if totalLessons > 0 && pct == 100 {
			stats.StudentsCompleted++
		}
	}
	if stats.StudentsTotal > 0 {
		stats.AvgPercent = sumPercent / stats.StudentsTotal
	}

	// Выручка — только успешные платежи: pending/failed/refunded не считаются
	// (фильтрацию по статусу делает handler, см. ListPaymentsByCourse), комиссия
	// не вычитается — сумма amountCents.
	for _, p := range a.Store.ListPaymentsByCourse(c.ID) {
		if p.Status == repo.PaymentSucceeded {
			stats.RevenueCents += p.AmountCents
		}
	}

	// Отзывы: среднее по рейтингам с округлением до 1 знака; без отзывов 0.
	reviews := a.Store.ListReviewsByCourse(c.ID)
	stats.ReviewsCount = len(reviews)
	if len(reviews) > 0 {
		sum := 0
		for _, rev := range reviews {
			sum += rev.Rating
		}
		stats.AvgRating = math.Round(float64(sum)/float64(len(reviews))*10) / 10
	}
	writeJSON(w, http.StatusOK, stats)
}
