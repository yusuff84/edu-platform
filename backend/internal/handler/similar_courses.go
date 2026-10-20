package handler

// T79 — похожие курсы на странице курса.
//
// GET /api/v1/courses/{id}/similar?limit=4 — публичный. Курс (UUID) должен
// существовать и быть published, иначе 404 «Курс не найден». Выдача —
// published-курсы той же category без самого курса: активные записи desc,
// avgRating desc, createdAt desc. Ответ — конверт {items:[CourseItem],total}.

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

const (
	// similarDefaultLimit — limit по умолчанию (контракт T79).
	similarDefaultLimit = 4
	// similarMaxLimit — максимум limit (контракт T79).
	similarMaxLimit = 12
)

// ListSimilarCourses — GET /courses/{id}/similar. Публичный маршрут:
// владение курс не требуется, но неопубликованный курс снаружи неотличим от
// несуществующего (404, как на странице курса).
//
// limit: нечисловой/<= 0 — 4, больше 12 — 12. Подборку и сортировку выполняет
// store.ListSimilarCourses одним вызовом; total — число отданных элементов
// (items — срез подборки после limit, контракт не обещает счётчик сверх него).
func (a *App) ListSimilarCourses(w http.ResponseWriter, r *http.Request) {
	courseID := chi.URLParam(r, "id")
	c, ok := a.Store.GetCourseByID(courseID)
	if !ok || c.Status != repo.CoursePublished {
		writeErr(w, http.StatusNotFound, "Курс не найден")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = similarDefaultLimit
	}
	if limit > similarMaxLimit {
		limit = similarMaxLimit
	}

	similar := a.Store.ListSimilarCourses(c.ID, limit)
	favs := a.favoriteCourseIDs(r)
	items := make([]courseItem, 0, len(similar))
	for _, sc := range similar {
		items = append(items, toCourseItemFor(sc, favs))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}
