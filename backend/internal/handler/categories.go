package handler

// T65 — категории курсов: фиксированный список слагов и публичный
// GET /categories. Порядок списка заморожен контрактом P8 и является
// порядком элементов ответа.

import (
	"net/http"
)

// courseCategories — все допустимые слаги категорий в фиксированном порядке
// (T65). Никакие другие значения API не принимает.
var courseCategories = []string{
	"programming", "design", "data", "marketing", "ai", "other",
}

// validCourseCategory проверяет слаг по фиксированному списку. Хелпер
// внутренний: наружу категории отдаются только через API.
func validCourseCategory(slug string) bool {
	for _, c := range courseCategories {
		if slug == c {
			return true
		}
	}
	return false
}

// categoryItem — элемент GET /categories: слаг и число опубликованных курсов.
type categoryItem struct {
	Slug         string `json:"slug"`
	CoursesCount int    `json:"coursesCount"`
}

// ListCategories — GET /categories (публичный, без авторизации): все шесть
// слагов в фиксированном порядке; coursesCount — только published-курсы,
// 0 допустим. items всегда массив (make, не nil).
func (a *App) ListCategories(w http.ResponseWriter, _ *http.Request) {
	counts := a.Store.CountPublishedCoursesByCategory()
	items := make([]categoryItem, 0, len(courseCategories))
	for _, slug := range courseCategories {
		items = append(items, categoryItem{Slug: slug, CoursesCount: counts[slug]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
