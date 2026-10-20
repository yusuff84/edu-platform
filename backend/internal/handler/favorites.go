package handler

// T73 — избранные курсы ученика (потребитель — T74):
//   PUT    /me/favorites/{courseId} — идемпотентно добавить published-курс;
//   DELETE /me/favorites/{courseId} — идемпотентно убрать;
//   GET    /me/favorites            — сохранённые курсы, новые первыми.
//
// Источник правды — Task/00-api-contract.md, раздел «T73 — Избранное»
// (заморожен). Признака isFavorite в публичных видах курса нет: клиент
// определяет избранное по этому списку. Элементы отдаются существующим
// toCourseItem (handler/courses.go), форма CourseItem не меняется.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// RegisterFavoritesRoutes вешает маршруты избранного на /api/v1: пути
// абсолютные (/me/*), requireAuth — здесь же через .With, как у остальных
// /me-маршрутов (см. middleware.go).
func (a *App) RegisterFavoritesRoutes(r chi.Router) {
	r.With(a.requireAuth).Put("/me/favorites/{courseId}", a.AddFavorite)
	r.With(a.requireAuth).Delete("/me/favorites/{courseId}", a.RemoveFavorite)
	r.With(a.requireAuth).Get("/me/favorites", a.MyFavorites)
}

// AddFavorite — PUT /me/favorites/{courseId}: только published-курс, иначе
// 404 «Курс не найден». Повторное добавление — тоже 204 (идемпотентно).
func (a *App) AddFavorite(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	courseID := strings.TrimSpace(chi.URLParam(r, "courseId"))
	c, found := a.Store.GetCourseByID(courseID)
	if !found || c.Status != repo.CoursePublished {
		// Неопубликованный курс не раскрываем: для ученика его не существует.
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}
	if err := a.Store.AddFavorite(u.ID, c.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Курс не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось добавить в избранное")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveFavorite — DELETE /me/favorites/{courseId}: 204 всегда, когда запрос
// авторизован, — в т.ч. если курса или пары уже нет (идемпотентно).
func (a *App) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	courseID := strings.TrimSpace(chi.URLParam(r, "courseId"))
	if err := a.Store.RemoveFavorite(u.ID, courseID); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить из избранного")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MyFavorites — GET /me/favorites: 200 {"items":[CourseItem],"total":N}.
// Порядок — добавленные позже первыми; архивные/неопубликованные курсы
// отфильтровывает хранилище. items всегда массив (не null).
func (a *App) MyFavorites(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	courses := a.Store.ListFavoriteCoursesByUser(u.ID)
	items := make([]courseItem, 0, len(courses))
	for _, c := range courses {
		items = append(items, toCourseItem(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}
