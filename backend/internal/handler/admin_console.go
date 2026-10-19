package handler

// T25 — админ-консоль супер-админа: сводка состояния платформы, список
// курсов всех статусов (архивация/снятие с публикации/возврат из архива),
// модерация отзывов и полный список записей на курсы.
// T48 — сортировка/фильтры курсов (sort/dir/teacherEmail/from/to) и записей
// (q/status), unarchive (archived → draft).
//
// Маршруты регистрируются внутри r.Route("/admin", ...) в middleware.go
// (RegisterAdminConsoleRoutes): requireAuth + requireRole(admin) навешаны
// родителем, пути относительные. Коды: 400 — валидация query, 409 — конфликт
// состояния (курс уже в архиве / не в архиве / не опубликован), 404 — нет
// сущности.
//
// JSON-поля — ровно по контракту T25 (фронтенд T27 пишется параллельно);
// items всегда массив (make, не nil), денежные суммы — в копейках.

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// RegisterAdminConsoleRoutes вешает маршруты T25 на /admin-роутер
// (см. middleware.go: вызов внутри r.Route("/admin", ...)).
//
// Все пути — ПОСЛЕ уже зарегистрированного GET /admin/courses/review:
// статический сегмент "review" в chi имеет приоритет над {id}, поэтому
// очередь модерации не затирается (проверено тестом TestAdminConsoleCoursesList,
// раздел «тень маршрута»).
func (a *App) RegisterAdminConsoleRoutes(r chi.Router) {
	r.Get("/stats", a.AdminStats)
	r.Get("/courses", a.AdminListCourses)
	r.Post("/courses/{id}/archive", a.AdminArchiveCourse)
	r.Post("/courses/{id}/unarchive", a.AdminUnarchiveCourse)
	r.Post("/courses/{id}/unpublish", a.AdminUnpublishCourse)
	r.Get("/courses/{id}/reviews", a.AdminCourseReviews)
	r.Delete("/reviews/{id}", a.AdminDeleteReview)
	r.Get("/enrollments", a.AdminEnrollments)
}

// ---- GET /admin/stats ----

type adminStatsUsers struct {
	Total    int `json:"total"`
	Students int `json:"students"`
	Teachers int `json:"teachers"`
	Admins   int `json:"admins"`
	Blocked  int `json:"blocked"`
	NewLast7 int `json:"newLast7"`
}

type adminStatsCourses struct {
	Total     int `json:"total"`
	Draft     int `json:"draft"`
	Review    int `json:"review"`
	Published int `json:"published"`
	Archived  int `json:"archived"`
}

type adminStatsPayments struct {
	Count            int `json:"count"`
	SucceededCents   int `json:"succeededCents"`
	PlatformFeeCents int `json:"platformFeeCents"`
	TeacherCents     int `json:"teacherCents"`
	Last30Cents      int `json:"last30Cents"`
}

type adminStatsEnrollments struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

type adminStatsReviews struct {
	Total     int     `json:"total"`
	AvgRating float64 `json:"avgRating"`
}

type adminStatsResponse struct {
	Users       adminStatsUsers       `json:"users"`
	Courses     adminStatsCourses     `json:"courses"`
	Payments    adminStatsPayments    `json:"payments"`
	Enrollments adminStatsEnrollments `json:"enrollments"`
	Reviews     adminStatsReviews     `json:"reviews"`
}

// AdminStats — GET /admin/stats: сводка состояния платформы (T25).
func (a *App) AdminStats(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	t := a.Store.AdminOverview()
	writeJSON(w, http.StatusOK, adminStatsResponse{
		Users: adminStatsUsers{
			Total: t.UsersTotal, Students: t.UsersStudents, Teachers: t.UsersTeachers,
			Admins: t.UsersAdmins, Blocked: t.UsersBlocked, NewLast7: t.UsersNew7,
		},
		Courses: adminStatsCourses{
			Total: t.CoursesTotal, Draft: t.CoursesDraft, Review: t.CoursesReview,
			Published: t.CoursesPublished, Archived: t.CoursesArchived,
		},
		Payments: adminStatsPayments{
			Count: t.PaymentsCount, SucceededCents: t.PaidCents,
			PlatformFeeCents: t.PlatformFeeCents, TeacherCents: t.TeacherCents,
			Last30Cents: t.PaidLast30Cents,
		},
		Enrollments: adminStatsEnrollments{Total: t.EnrollmentsTotal, Active: t.EnrollmentsActive},
		Reviews:     adminStatsReviews{Total: t.ReviewsTotal, AvgRating: t.AvgRating},
	})
}

// ---- GET /admin/courses ----

// adminCourseItem — строка списка курсов в форме контракта T25: карточка
// без description/currency и счётчиками программы и учеников.
type adminCourseItem struct {
	ID            string            `json:"id"`
	Slug          string            `json:"slug"`
	Title         string            `json:"title"`
	CoverPath     *string           `json:"coverPath"`
	TeacherID     string            `json:"teacherId"`
	TeacherEmail  string            `json:"teacherEmail"`
	Status        repo.CourseStatus `json:"status"`
	PriceCents    int               `json:"priceCents"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	ModulesCount  int               `json:"modulesCount"`
	LessonsCount  int               `json:"lessonsCount"`
	StudentsCount int               `json:"studentsCount"`
}

type adminCourseListResponse struct {
	Items []adminCourseItem `json:"items"`
	Total int               `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}

// AdminListCourses — GET /admin/courses?status=&q=&page=&limit=
// (T25) + sort=&dir=&teacherEmail=&from=&to= (T48): курсы всех статусов.
// 400 — статус вне draft|review|published|archived, нечисловые page/limit,
// sort вне createdAt|updatedAt|title|price|students, dir вне asc|desc,
// мусорные from/to или from > to. Счётчики — одним вызовом
// CourseContentCounts (без N+1); teacherEmail — из справочника пользователей
// (с кэшем по id).
func (a *App) AdminListCourses(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}
	q := r.URL.Query()
	status := repo.CourseStatus(strings.TrimSpace(q.Get("status")))
	switch status {
	case "", repo.CourseDraft, repo.CourseReview, repo.CoursePublished, repo.CourseArchived:
	default:
		writeErr(w, http.StatusBadRequest,
			"Некорректный статус: допустимы draft, review, published, archived")
		return
	}
	// T48: сортировка и направление. Пустые значения — дефолты контракта
	// (createdAt/desc), поэтому старые запросы работают как раньше.
	sortParam := strings.TrimSpace(q.Get("sort"))
	switch sortParam {
	case "", "createdAt", "updatedAt", "title", "price", "students":
	default:
		writeErr(w, http.StatusBadRequest,
			"Параметр sort должен быть одним из: createdAt, updatedAt, title, price, students")
		return
	}
	dirParam := strings.TrimSpace(q.Get("dir"))
	switch dirParam {
	case "", "asc", "desc":
	default:
		writeErr(w, http.StatusBadRequest, "Параметр dir должен быть asc или desc")
		return
	}
	// T48: период по createdAt — тем же парсером, что журнал платежей (T31);
	// тексты ошибок («Дата в формате ГГГГ-ММ-ДД», «Начало периода позже конца»)
	// приходят оттуда.
	from, to, ok := parseJournalPeriod(w, r)
	if !ok {
		return
	}

	items, total := a.Store.ListAllCourses(repo.CourseFilter{
		Page: page, Limit: limit, Status: status, Q: q.Get("q"),
		Sort: sortParam, Dir: dirParam,
		TeacherEmail: strings.TrimSpace(q.Get("teacherEmail")),
		From:         from, To: to,
	})
	counts := a.Store.CourseContentCounts()
	emails := make(map[string]string)
	out := make([]adminCourseItem, 0, len(items))
	for _, c := range items {
		email, cached := emails[c.TeacherID]
		if !cached {
			if u, ok := a.Store.GetUserByID(c.TeacherID); ok {
				email = u.Email
			}
			emails[c.TeacherID] = email
		}
		cc := counts[c.ID]
		out = append(out, adminCourseItem{
			ID: c.ID, Slug: c.Slug, Title: c.Title, CoverPath: c.CoverPath,
			TeacherID: c.TeacherID, TeacherEmail: email, Status: c.Status,
			PriceCents: c.PriceCents, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
			ModulesCount: cc.Modules, LessonsCount: cc.Lessons, StudentsCount: cc.Students,
		})
	}
	writeJSON(w, http.StatusOK, adminCourseListResponse{
		Items: out, Total: total, Page: page, Limit: limit,
	})
}

// ---- POST /admin/courses/{id}/archive|unarchive|unpublish ----

// AdminArchiveCourse — POST /admin/courses/{id}/archive (T25): 200
// {"course": {...}} со статусом archived; 409 — курс уже archived; 404 — нет
// курса. Комментарий модерации сохраняется.
func (a *App) AdminArchiveCourse(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if c.Status == repo.CourseArchived {
		writeErr(w, http.StatusConflict, "Курс уже в архиве")
		return
	}
	a.Store.SetCourseStatus(c.ID, repo.CourseArchived, c.ModerationComment)
	updated, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, map[string]any{"course": updated})
}

// AdminUnpublishCourse — POST /admin/courses/{id}/unpublish (T25): 200
// {"course": {...}} со статусом draft (published → draft); 409 — любой
// другой статус; 404 — нет курса.
func (a *App) AdminUnpublishCourse(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if c.Status != repo.CoursePublished {
		writeErr(w, http.StatusConflict, "Снять с публикации можно только опубликованный курс")
		return
	}
	a.Store.SetCourseStatus(c.ID, repo.CourseDraft, c.ModerationComment)
	updated, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, map[string]any{"course": updated})
}

// AdminUnarchiveCourse — POST /admin/courses/{id}/unarchive (T48): 200
// {"course": {...}} со статусом draft (archived → draft, как у
// /teacher/courses/{id}/unarchive из T21); 409 — курс не в архиве; 404 — нет
// курса. Комментарий модерации сохраняется: SetCourseStatus получает текущее
// значение, а не пустую строку.
func (a *App) AdminUnarchiveCourse(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if c.Status != repo.CourseArchived {
		writeErr(w, http.StatusConflict, "Вернуть из архива можно только архивный курс")
		return
	}
	a.Store.SetCourseStatus(c.ID, repo.CourseDraft, c.ModerationComment)
	updated, _ := a.Store.GetCourseByID(c.ID)
	writeJSON(w, http.StatusOK, map[string]any{"course": updated})
}

// ---- GET /admin/courses/{id}/reviews ----

// adminReviewItem — отзыв в форме контракта T25: с email ученика, без
// courseId (курс уже известен из пути).
type adminReviewItem struct {
	ID           string    `json:"id"`
	StudentID    string    `json:"studentId"`
	StudentEmail string    `json:"studentEmail"`
	Rating       int       `json:"rating"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"createdAt"`
	// T69: аддитивные поля ответа преподавателя (пустая строка/null — нет).
	TeacherReply   string     `json:"teacherReply"`
	TeacherReplyAt *time.Time `json:"teacherReplyAt"`
}

type adminReviewListResponse struct {
	Items []adminReviewItem `json:"items"`
}

// AdminCourseReviews — GET /admin/courses/{id}/reviews (T25): отзывы курса
// для модерации, новые первыми; 404 — нет курса; items — всегда массив.
func (a *App) AdminCourseReviews(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	c, found := a.Store.GetCourseByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !found {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	list := a.Store.ListReviewsByCourse(c.ID)
	items := make([]adminReviewItem, 0, len(list))
	for _, rv := range list {
		email := ""
		if u, ok := a.Store.GetUserByID(rv.StudentID); ok {
			email = u.Email
		}
		items = append(items, adminReviewItem{
			ID: rv.ID, StudentID: rv.StudentID, StudentEmail: email,
			Rating: rv.Rating, Body: rv.Body, CreatedAt: rv.CreatedAt,
			TeacherReply: rv.TeacherReply, TeacherReplyAt: rv.TeacherReplyAt,
		})
	}
	writeJSON(w, http.StatusOK, adminReviewListResponse{Items: items})
}

// ---- DELETE /admin/reviews/{id} ----

// AdminDeleteReview — DELETE /admin/reviews/{id} (T25): 204 без тела; после
// удаления рейтинг курса пересчитан (делает Store.DeleteReview). 404 — нет
// отзыва (в том числе мусорный id: «сущности нет», а не 500 из БД).
func (a *App) AdminDeleteReview(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if err := a.Store.DeleteReview(id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Отзыв не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить отзыв")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- GET /admin/enrollments ----

// adminEnrollmentItem — запись на курс в форме контракта T25: данные
// ученика/курса и прогресс в уроках и процентах.
type adminEnrollmentItem struct {
	ID               string                `json:"id"`
	StudentID        string                `json:"studentId"`
	StudentEmail     string                `json:"studentEmail"`
	StudentName      string                `json:"studentName"`
	CourseID         string                `json:"courseId"`
	CourseTitle      string                `json:"courseTitle"`
	Status           repo.EnrollmentStatus `json:"status"`
	CreatedAt        time.Time             `json:"createdAt"`
	CompletedLessons int                   `json:"completedLessons"`
	TotalLessons     int                   `json:"totalLessons"`
	Percent          int                   `json:"percent"`
}

type adminEnrollmentListResponse struct {
	Items []adminEnrollmentItem `json:"items"`
	Total int                   `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
}

// AdminEnrollments — GET /admin/enrollments?courseId=&studentId=&q=&status=
// &page=&limit= (T25/T48): все записи на курсы, новые первыми; фильтры
// комбинируются через AND; 400 — нечисловая пагинация, courseId/studentId
// не UUID, status вне active|refunded|revoked. T48: q — подстрока без учёта
// регистра по email ИЛИ displayName ученика.
func (a *App) AdminEnrollments(w http.ResponseWriter, r *http.Request) {
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
	studentID := strings.TrimSpace(q.Get("studentId"))
	if studentID != "" {
		if _, err := uuid.Parse(studentID); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр studentId должен быть UUID")
			return
		}
	}
	// T48: статус записи — точное совпадение; пусто — любой.
	status := repo.EnrollmentStatus(strings.TrimSpace(q.Get("status")))
	switch status {
	case "", repo.EnrollmentActive, repo.EnrollmentRefunded, repo.EnrollmentRevoked:
	default:
		writeErr(w, http.StatusBadRequest,
			"Некорректный статус записи: допустимы active, refunded, revoked")
		return
	}

	rows, total := a.Store.ListEnrollmentRows(repo.EnrollmentFilter{
		Page: page, Limit: limit, CourseID: courseID, StudentID: studentID,
		Q: strings.TrimSpace(q.Get("q")), Status: status,
	})
	items := make([]adminEnrollmentItem, 0, len(rows))
	for _, row := range rows {
		percent := 0
		if row.TotalLessons > 0 {
			percent = row.Completed * 100 / row.TotalLessons
		}
		items = append(items, adminEnrollmentItem{
			ID: row.ID, StudentID: row.StudentID,
			StudentEmail: row.StudentEmail, StudentName: row.StudentName,
			CourseID: row.CourseID, CourseTitle: row.CourseTitle,
			Status: row.Status, CreatedAt: row.CreatedAt,
			CompletedLessons: row.Completed, TotalLessons: row.TotalLessons,
			Percent: percent,
		})
	}
	writeJSON(w, http.StatusOK, adminEnrollmentListResponse{
		Items: items, Total: total, Page: page, Limit: limit,
	})
}
