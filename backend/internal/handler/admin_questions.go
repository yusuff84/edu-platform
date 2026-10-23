package handler

// T85 — глобальная модерация Q&A (волна P10).
// Контракт — Task/00-api-contract.md, раздел «T85 — GET /admin/questions»
// (заморожен): только admin, фильтры courseId/lessonId/q, пагинация
// page/limit, ответ {items,total,page,limit}. Удаление чужих вопросов и
// ответов выполняют существующие маршруты T71 (lesson_qa.go): admin уже
// входит в canModerateQA.

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// RegisterAdminQuestionsRoutes регистрирует GET /questions внутри группы
// /admin (requireAuth + requireRole(admin) уже навешены в middleware.go).
func (a *App) RegisterAdminQuestionsRoutes(r chi.Router) {
	r.Get("/questions", a.AdminListQuestions)
}

// adminQuestionsPage — конверт GET /admin/questions: items (всегда массив),
// total до пагинации и эхо page/limit.
type adminQuestionsPage struct {
	Items []*repo.AdminQuestionRow `json:"items"`
	Total int                      `json:"total"`
	Page  int                      `json:"page"`
	Limit int                      `json:"limit"`
}

// parseAdminQuestionsPagination разбирает page/limit T85. Пустые значения —
// дефолты (1 и 20). Нечисловые, page < 1, limit < 1 или limit > 100 — 400
// (контракт: «неверная пагинация — 400»). Текст ошибки — в стиле остальных
// админ-списков.
func parseAdminQuestionsPagination(w http.ResponseWriter, r *http.Request) (page, limit int, ok bool) {
	q := r.URL.Query()
	page, limit = 1, 20

	if raw := strings.TrimSpace(q.Get("page")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
			return 0, 0, false
		}
		if n < 1 {
			writeErr(w, http.StatusBadRequest, "Параметр page должен быть положительным целым числом")
			return 0, 0, false
		}
		page = n
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
			return 0, 0, false
		}
		if n < 1 || n > 100 {
			writeErr(w, http.StatusBadRequest, "Параметр limit должен быть целым числом от 1 до 100")
			return 0, 0, false
		}
		limit = n
	}
	return page, limit, true
}

// AdminListQuestions — GET /admin/questions: глобальный список Q&A для
// модерации. Фильтры: courseId (UUID), lessonId (UUID), q (поиск без учёта
// регистра по вопросам/ответам/email/именам/названиям курса и урока).
// items — новые вопросы первыми, ответы внутри — старые первыми; total — до
// пагинации. Неверные UUID/пагинация — 400.
func (a *App) AdminListQuestions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	courseID := strings.TrimSpace(q.Get("courseId"))
	if courseID != "" {
		if _, err := uuid.Parse(courseID); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр courseId должен быть UUID")
			return
		}
	}
	lessonID := strings.TrimSpace(q.Get("lessonId"))
	if lessonID != "" {
		if _, err := uuid.Parse(lessonID); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр lessonId должен быть UUID")
			return
		}
	}
	page, limit, ok := parseAdminQuestionsPagination(w, r)
	if !ok {
		return
	}

	items, total := a.Store.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: courseID,
		LessonID: lessonID,
		Q:        strings.TrimSpace(q.Get("q")),
		Page:     page,
		Limit:    limit,
	})
	if items == nil {
		items = []*repo.AdminQuestionRow{}
	}
	writeJSON(w, http.StatusOK, adminQuestionsPage{
		Items: items, Total: total, Page: page, Limit: limit,
	})
}
