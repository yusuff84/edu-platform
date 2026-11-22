package handler

// Новая админка: журнал действий администраторов, сводка «что требует внимания»
// и сквозные списки по всем курсам (вопросы к урокам, отзывы, промокоды).
//
//   GET /admin/overview     — сводка для главной админки
//   GET /admin/audit        — журнал действий (q, page, limit)
//   GET /admin/questions    — вопросы к урокам всех курсов (status=unanswered|answered|all, courseId)
//   GET /admin/reviews      — отзывы всех курсов (rating, q, courseId)
//   GET /admin/promo-codes  — промокоды всех курсов (courseId)
//
// Ответы на вопросы, ответы на отзывы и создание/отключение промокодов идут
// через существующие маршруты (администратор проходит проверки владельца курса).

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"edu-platform/backend/internal/repo"
)

// ---------- журнал действий ----------

const auditCtxKey ctxKey = "admin-audit"

// auditHolder собирает подробности действия за время обработки запроса:
// обработчики дописывают их через auditAdmin, middleware пишет одну запись.
type auditHolder struct {
	mu      sync.Mutex
	actor   *repo.User
	details []string
}

func (h *auditHolder) add(u *repo.User, msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if u != nil && h.actor == nil {
		h.actor = u
	}
	h.details = append(h.details, msg)
}

// auditAdmin фиксирует действие администратора: внутри запроса — через
// middleware (одна запись на запрос), вне его — сразу в журнал.
func (a *App) auditAdmin(r *http.Request, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u, _ := a.currentUser(r)
	who := "?"
	if u != nil {
		who = u.ID
	}
	log.Printf("admin-audit %s: %s", who, msg)
	if h, ok := r.Context().Value(auditCtxKey).(*auditHolder); ok {
		h.add(u, msg)
		return
	}
	if u != nil {
		a.Store.AddAuditEntry(u.ID, u.Email, msg, clientIP(r))
	}
}

// mcpToolLabels — подписи меняющих инструментов MCP для журнала.
var mcpToolLabels = map[string]string{
	"create_course":    "создан курс",
	"update_course":    "изменён курс",
	"add_module":       "добавлен модуль",
	"rename_module":    "переименован модуль",
	"delete_module":    "удалён модуль",
	"add_lesson":       "добавлен урок",
	"update_lesson":    "изменён урок",
	"delete_lesson":    "удалён урок",
	"publish_course":   "опубликован курс",
	"unpublish_course": "курс снят с публикации",
	"delete_course":    "удалён курс",
	"set_course_cover": "обновлена обложка курса",
}

// auditMCP записывает успешный меняющий вызов инструмента MCP в журнал действий.
func (a *App) auditMCP(ctx context.Context, u *repo.User, tool string, args map[string]any, isErr bool) {
	log.Printf("admin-audit %s: mcp tool %s error=%v", u.ID, tool, isErr)
	label, ok := mcpToolLabels[tool]
	if isErr || !ok {
		return
	}
	msg := "Нейросеть (MCP): " + label
	title, _ := args["title"].(string)
	if title != "" {
		msg += " «" + title + "»"
	}
	if cid, _ := args["course_id"].(string); cid != "" && tool != "create_course" {
		if c, found := a.Store.GetCourseByID(cid); found {
			if strings.HasSuffix(tool, "_course") {
				if title == "" {
					msg += " «" + c.Title + "»"
				}
			} else {
				msg += " в курсе «" + c.Title + "»"
			}
		}
	}
	if h, ok := ctx.Value(auditCtxKey).(*auditHolder); ok {
		// Одна запись на вызов: подписи вложенных обработчиков заменяет подпись MCP.
		h.mu.Lock()
		h.details = nil
		h.mu.Unlock()
		h.add(u, msg)
		return
	}
	a.Store.AddAuditEntry(u.ID, u.Email, msg, "")
}

// auditLabels — подписи меняющих маршрутов /admin без собственного auditAdmin.
var auditLabels = map[string]string{
	"POST /api/v1/admin/invites":                                             "Отправлено приглашение преподавателю",
	"POST /api/v1/admin/invites/{id}/revoke":                                 "Отозвано приглашение преподавателя",
	"POST /api/v1/admin/courses/{id}/moderate":                               "Решение по модерации курса",
	"PATCH /api/v1/admin/users/{id}/role":                                    "Изменена роль пользователя",
	"PATCH /api/v1/admin/users/{id}/status":                                  "Изменён статус пользователя",
	"DELETE /api/v1/admin/users/{id}":                                        "Удалён пользователь",
	"POST /api/v1/admin/courses/{id}/archive":                                "Курс отправлен в архив",
	"POST /api/v1/admin/courses/{id}/unarchive":                              "Курс возвращён из архива",
	"POST /api/v1/admin/courses/{id}/unpublish":                              "Курс снят с публикации",
	"POST /api/v1/admin/courses/{id}/withdraw":                               "Курс снят с модерации",
	"POST /api/v1/admin/courses/{id}/duplicate":                              "Создана копия курса",
	"POST /api/v1/admin/courses/{id}/lessons/{lessonId}/move":                "Урок перенесён",
	"DELETE /api/v1/admin/reviews/{id}":                                      "Удалён отзыв",
	"POST /api/v1/admin/payments/{id}/refund":                                "Оформлен возврат платежа",
	"POST /api/v1/admin/payments/{id}/sync":                                  "Статус платежа сверен с провайдером",
	"PUT /api/v1/admin/settings":                                             "Изменены настройки платформы",
	"POST /api/v1/admin/enrollments/grant":                                   "Выдан доступ к курсу",
	"POST /api/v1/admin/enrollments/{studentId}/courses/{courseId}/revoke":   "Закрыт доступ к курсу",
	"POST /api/v1/admin/enrollments/{studentId}/courses/{courseId}/unrevoke": "Возвращён доступ к курсу",
	"POST /api/v1/admin/leads/{id}/status":                                   "Изменён статус заявки",
}

// adminAudit пишет в журнал успешные меняющие запросы администраторов.
func (a *App) adminAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		h := &auditHolder{}
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		r = r.WithContext(context.WithValue(r.Context(), auditCtxKey, h))
		// Цель действия описываем до обработчика: после удаления её уже не найти.
		var target string
		var pattern string
		if strings.Contains(r.URL.Path, "/admin/") {
			target = a.auditTarget(r)
		}
		next.ServeHTTP(ww, r)
		if st := ww.Status(); st >= 400 {
			return
		}
		if rc := chi.RouteContext(r.Context()); rc != nil {
			pattern = rc.RoutePattern()
		}
		action := strings.Join(h.details, "; ")
		if action == "" {
			label, ok := auditLabels[r.Method+" "+pattern]
			if !ok {
				if !strings.HasPrefix(pattern, "/api/v1/admin/") {
					return
				}
				label = r.Method + " " + strings.TrimPrefix(pattern, "/api/v1")
			}
			action = label
			if extra := a.auditAfter(r, pattern); extra != "" {
				action += ": " + extra
			} else if target != "" {
				action += ": " + target
			}
		}
		actor := h.actor
		if actor == nil {
			actor = a.optionalUser(r)
		}
		if actor == nil || actor.Role != repo.RoleAdmin {
			return
		}
		a.Store.AddAuditEntry(actor.ID, actor.Email, action, clientIP(r))
	})
}

// auditTarget описывает объект запроса по параметрам пути (email, название курса).
func (a *App) auditTarget(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/")
	parts := strings.Split(path, "/")
	var out []string
	for i := 0; i+1 < len(parts); i++ {
		id := parts[i+1]
		switch parts[i] {
		case "users", "enrollments":
			if u, ok := a.Store.GetUserByID(id); ok {
				out = append(out, u.Email)
			}
		case "courses":
			if c, ok := a.Store.GetCourseByID(id); ok {
				out = append(out, "«"+c.Title+"»")
			}
		}
	}
	return strings.Join(out, ", ")
}

// auditAfter — подробности, известные только после обработки (новая роль, статус).
func (a *App) auditAfter(r *http.Request, pattern string) string {
	id := chi.URLParamFromCtx(r.Context(), "id")
	switch pattern {
	case "/api/v1/admin/users/{id}/role":
		if u, ok := a.Store.GetUserByID(id); ok {
			return fmt.Sprintf("%s → %s", u.Email, roleRu(u.Role))
		}
	case "/api/v1/admin/users/{id}/status":
		if u, ok := a.Store.GetUserByID(id); ok {
			if u.Status == repo.UserBlocked {
				return u.Email + " заблокирован"
			}
			return u.Email + " разблокирован"
		}
	case "/api/v1/admin/courses/{id}/moderate":
		if c, ok := a.Store.GetCourseByID(id); ok {
			if c.Status == repo.CoursePublished {
				return "«" + c.Title + "» одобрен и опубликован"
			}
			return "«" + c.Title + "» отклонён"
		}
	}
	return ""
}

func roleRu(r repo.Role) string {
	switch r {
	case repo.RoleAdmin:
		return "администратор"
	case repo.RoleTeacher:
		return "преподаватель"
	default:
		return "ученик"
	}
}

// RegisterAdminHubRoutes — маршруты новой админки (вызывается на уровне /api/v1).
func (a *App) RegisterAdminHubRoutes(r chi.Router) {
	adm := r.With(a.requireAuth, a.requireRole(repo.RoleAdmin))
	adm.Get("/admin/overview", a.AdminOverview)
	adm.Get("/admin/audit", a.AdminAuditLog)
	adm.Get("/admin/questions", a.AdminAllQuestions)
	adm.Get("/admin/reviews", a.AdminAllReviews)
	adm.Get("/admin/promo-codes", a.AdminAllPromoCodes)
}

func hubPageLimit(r *http.Request, def int) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = def
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

func pageSlice[T any](items []T, page, limit int) []T {
	start := (page - 1) * limit
	if start >= len(items) {
		return []T{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// AdminAuditLog — GET /admin/audit.
func (a *App) AdminAuditLog(w http.ResponseWriter, r *http.Request) {
	page, limit := hubPageLimit(r, 30)
	items, total := a.Store.ListAuditEntries(r.URL.Query().Get("q"), page, limit)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "limit": limit})
}

// allCourses — все курсы платформы (постранично из ListAllCourses).
func (a *App) allCourses() []*repo.Course {
	out := make([]*repo.Course, 0)
	for page := 1; page <= 100; page++ {
		items, total := a.Store.ListAllCourses(repo.CourseFilter{Page: page, Limit: 100})
		out = append(out, items...)
		if len(items) == 0 || len(out) >= total {
			break
		}
	}
	return out
}

// ---------- вопросы к урокам ----------

type adminQuestionItem struct {
	*repo.LessonQuestion
	CourseTitle  string `json:"courseTitle"`
	CourseSlug   string `json:"courseSlug"`
	StudentEmail string `json:"studentEmail"`
	Answered     bool   `json:"answered"`
}

func (a *App) allQuestions(courseID string) []adminQuestionItem {
	out := make([]adminQuestionItem, 0)
	emails := map[string]string{}
	for _, c := range a.allCourses() {
		if courseID != "" && c.ID != courseID {
			continue
		}
		items, _ := a.Store.ListLessonQuestions(c.ID, "", 1, 1000)
		for _, q := range items {
			answered := false
			for _, ans := range q.Answers {
				if ans.IsTeacher {
					answered = true
					break
				}
			}
			email, ok := emails[q.StudentID]
			if !ok {
				if u, found := a.Store.GetUserByID(q.StudentID); found {
					email = u.Email
				}
				emails[q.StudentID] = email
			}
			out = append(out, adminQuestionItem{LessonQuestion: q, CourseTitle: c.Title, CourseSlug: c.Slug, StudentEmail: email, Answered: answered})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// AdminAllQuestions — GET /admin/questions.
func (a *App) AdminAllQuestions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	page, limit := hubPageLimit(r, 20)
	all := a.allQuestions(q.Get("courseId"))
	items := make([]adminQuestionItem, 0, len(all))
	unanswered := 0
	for _, it := range all {
		if !it.Answered {
			unanswered++
		}
		if (status == "unanswered" && it.Answered) || (status == "answered" && !it.Answered) {
			continue
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": pageSlice(items, page, limit), "total": len(items), "unanswered": unanswered, "page": page, "limit": limit,
	})
}

// ---------- отзывы ----------

type hubReviewItem struct {
	*repo.Review
	CourseTitle  string `json:"courseTitle"`
	CourseSlug   string `json:"courseSlug"`
	StudentEmail string `json:"studentEmail"`
	StudentName  string `json:"studentName"`
}

// AdminAllReviews — GET /admin/reviews.
func (a *App) AdminAllReviews(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, limit := hubPageLimit(r, 20)
	rating, _ := strconv.Atoi(q.Get("rating"))
	needle := strings.ToLower(strings.TrimSpace(q.Get("q")))
	courseID := q.Get("courseId")
	noReply := q.Get("noReply") == "1"
	items := make([]hubReviewItem, 0)
	for _, c := range a.allCourses() {
		if courseID != "" && c.ID != courseID {
			continue
		}
		for _, rv := range a.Store.ListReviewsByCourse(c.ID) {
			if rating > 0 && rv.Rating != rating {
				continue
			}
			if noReply && rv.TeacherReply != "" {
				continue
			}
			it := hubReviewItem{Review: rv, CourseTitle: c.Title, CourseSlug: c.Slug}
			if u, ok := a.Store.GetUserByID(rv.StudentID); ok {
				it.StudentEmail, it.StudentName = u.Email, u.DisplayName
			}
			if needle != "" && !strings.Contains(strings.ToLower(rv.Body+" "+it.StudentEmail+" "+it.StudentName+" "+c.Title), needle) {
				continue
			}
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"items": pageSlice(items, page, limit), "total": len(items), "page": page, "limit": limit})
}

// ---------- промокоды ----------

type adminPromoItem struct {
	*repo.PromoCode
	CourseTitle string `json:"courseTitle"`
	CoursePrice int    `json:"coursePriceCents"`
}

// AdminAllPromoCodes — GET /admin/promo-codes.
func (a *App) AdminAllPromoCodes(w http.ResponseWriter, r *http.Request) {
	courseID := r.URL.Query().Get("courseId")
	items := make([]adminPromoItem, 0)
	for _, c := range a.allCourses() {
		if courseID != "" && c.ID != courseID {
			continue
		}
		for _, p := range a.Store.ListPromoCodesByCourse(c.ID) {
			items = append(items, adminPromoItem{PromoCode: p, CourseTitle: c.Title, CoursePrice: c.PriceCents})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// ---------- сводка ----------

type overviewDay struct {
	Date         string `json:"date"`
	Signups      int    `json:"signups"`
	Enrollments  int    `json:"enrollments"`
	RevenueCents int    `json:"revenueCents"`
	Payments     int    `json:"payments"`
}

type overviewUser struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        repo.Role `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}

type overviewPayment struct {
	ID           string             `json:"id"`
	StudentID    string             `json:"studentId"`
	StudentEmail string             `json:"studentEmail"`
	CourseID     string             `json:"courseId"`
	CourseTitle  string             `json:"courseTitle"`
	AmountCents  int                `json:"amountCents"`
	Status       repo.PaymentStatus `json:"status"`
	CreatedAt    time.Time          `json:"createdAt"`
}

// mskZone — сутки сводки считаются по Москве.
var mskZone = time.FixedZone("MSK", 3*3600)

// AdminOverview — GET /admin/overview?days=30.
func (a *App) AdminOverview(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 7 || days > 90 {
		days = 30
	}
	now := time.Now().In(mskZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, mskZone)
	since := today.AddDate(0, 0, -(days - 1))
	series := make([]overviewDay, days)
	idx := map[string]int{}
	for i := range series {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		series[i].Date = d
		idx[d] = i
	}
	bump := func(t time.Time, f func(*overviewDay)) {
		if i, ok := idx[t.In(mskZone).Format("2006-01-02")]; ok {
			f(&series[i])
		}
	}

	users := a.Store.ListUsers(repo.UserFilter{})
	sort.Slice(users, func(i, j int) bool { return users[i].CreatedAt.After(users[j].CreatedAt) })
	recentUsers := make([]overviewUser, 0, 6)
	for i, u := range users {
		bump(u.CreatedAt, func(d *overviewDay) { d.Signups++ })
		if i < 6 {
			recentUsers = append(recentUsers, overviewUser{u.ID, u.Email, u.DisplayName, u.Role, u.CreatedAt})
		}
	}

	for page := 1; page <= 200; page++ {
		rows, total := a.Store.ListEnrollmentRows(repo.EnrollmentFilter{Page: page, Limit: 100})
		for _, e := range rows {
			bump(e.CreatedAt, func(d *overviewDay) { d.Enrollments++ })
		}
		if len(rows) == 0 || page*100 >= total {
			break
		}
	}

	pending, failed7 := 0, 0
	recentPayments := make([]overviewPayment, 0, 6)
	for page := 1; page <= 200; page++ {
		rows, total, _, err := a.Store.ListPaymentsJournal(repo.PaymentFilter{Page: page, Limit: 100, From: since})
		if err != nil {
			break
		}
		for _, p := range rows {
			if p.Status == repo.PaymentSucceeded {
				bump(p.CreatedAt, func(d *overviewDay) { d.RevenueCents += p.AmountCents; d.Payments++ })
			}
			if p.Status == repo.PaymentPending {
				pending++
			}
			if p.Status == repo.PaymentFailed && p.CreatedAt.After(now.AddDate(0, 0, -7)) {
				failed7++
			}
			if len(recentPayments) < 6 {
				recentPayments = append(recentPayments, overviewPayment{p.ID, p.StudentID, p.StudentEmail, p.CourseID, p.CourseTitle, p.AmountCents, p.Status, p.CreatedAt})
			}
		}
		if len(rows) == 0 || page*100 >= total {
			break
		}
	}

	newLeads := 0
	for _, l := range a.Store.ListLeads(1000) {
		if l.Status == repo.LeadStatusNew {
			newLeads++
		}
	}
	unanswered := 0
	for _, q := range a.allQuestions("") {
		if !q.Answered {
			unanswered++
		}
	}
	noReply := 0
	lowRating := 0
	for _, c := range a.allCourses() {
		for _, rv := range a.Store.ListReviewsByCourse(c.ID) {
			if rv.TeacherReply == "" {
				noReply++
				if rv.Rating <= 3 {
					lowRating++
				}
			}
		}
	}
	audit, _ := a.Store.ListAuditEntries("", 1, 8)

	writeJSON(w, http.StatusOK, map[string]any{
		"attention": map[string]int{
			"newLeads":            newLeads,
			"reviewQueue":         len(a.Store.ListCoursesByStatus(repo.CourseReview)),
			"unansweredQuestions": unanswered,
			"pendingPayments":     pending,
			"failedPayments7d":    failed7,
			"reviewsNoReply":      noReply,
			"lowRatingNoReply":    lowRating,
		},
		"series":         series,
		"recentUsers":    recentUsers,
		"recentPayments": recentPayments,
		"recentAudit":    audit,
	})
}
