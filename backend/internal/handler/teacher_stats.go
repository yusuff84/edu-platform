package handler

// T77 — аналитика учителя (волна P9; потребитель — фронтенд T78).
//
// Маршруты регистрируются RegisterTeacherStatsRoutes внутри
// r.Route("/teacher", ...) в middleware.go: requireAuth + requireRole(teacher,
// admin) уже навешаны родительской группой, пути относительные.
//
// Права: teacher видит только свои курсы, admin — все курсы платформы
// (флаг all в вызовах StoreAPI). Форма ответов — ровно по замороженному
// контракту P9 (Task/00-api-contract.md, раздел T77).

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterTeacherStatsRoutes вешает маршруты аналитики на /teacher-роутер
// (см. middleware.go: вызов внутри r.Route("/teacher", ...)).
func (a *App) RegisterTeacherStatsRoutes(r chi.Router) {
	r.Get("/stats", a.TeacherStats)
	r.Get("/stats/courses", a.TeacherStatsCourses)
}

// teacherStatsAll — admin видит все курсы платформы, teacher — только свои.
func teacherStatsAll(u *repo.User) bool {
	return u.Role == repo.RoleAdmin
}

// parseTeacherStatsLimit разбирает limit разбивки по курсам: по умолчанию 50,
// максимум 200. Нечисловое, нулевое и отрицательное значения трактуются как
// отсутствие параметра (дефолт), как в контракте P9 для limit-параметров;
// больше 200 — обрезается до 200.
func parseTeacherStatsLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

// TeacherStats — GET /teacher/stats: сводка преподавателя (курсы по
// статусам, ученики, выручка, завершаемость, рейтинг, последние 30 дней).
// teacher — только свои курсы, admin — все. 401 — нет сессии (страховка:
// requireAuth уже отсекает), 500 — сбой БД в Postgres-режиме.
func (a *App) TeacherStats(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	overview, err := a.Store.TeacherOverview(u.ID, teacherStatsAll(u))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось получить аналитику")
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

// teacherStatsCoursesResponse — конверт GET /teacher/stats/courses: items
// всегда массив (не null), total — число возвращённых элементов (после
// применения limit; пагинации у эндпоинта нет).
type teacherStatsCoursesResponse struct {
	Items []*repo.TeacherCourseStat `json:"items"`
	Total int                       `json:"total"`
}

// TeacherStatsCourses — GET /teacher/stats/courses?limit=50: разбивка по
// курсам любого статуса. Порядок и агрегаты — по контракту T77, limit
// (дефолт 50, максимум 200) применяет хранилище. teacher — только свои курсы,
// admin — все.
func (a *App) TeacherStatsCourses(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	limit := parseTeacherStatsLimit(r.URL.Query().Get("limit"))
	items := a.Store.TeacherCourseStats(u.ID, teacherStatsAll(u), limit)
	if items == nil {
		items = []*repo.TeacherCourseStat{}
	}
	writeJSON(w, http.StatusOK, teacherStatsCoursesResponse{Items: items, Total: len(items)})
}
