package repo

// T83 — глобальная модерация отзывов: типы фильтра и строки списка отзывов
// для админки. Файл содержит объявления типов и общие помощники; in-memory
// реализация — в store_admin_reviews.go, SQL — в postgres_admin_reviews.go.
//
// Модели (models.go) намеренно не трогаются: задача P10 не должна менять
// общие типы, которые параллельно используют другие задачи волны.

import (
	"sort"
	"strings"
	"time"
)

// AdminReviewFilter — параметры глобального списка отзывов (T83).
//
// CourseID "" — любой курс, иначе точное совпадение UUID. Rating nil — без
// фильтра, иначе точное значение 1..5. Q — подстрока без учёта регистра по
// названию курса, email/имени ученика и тексту отзыва; пусто — без поиска.
// From/To — границы по createdAt включительно (нулевое значение — границы
// нет; To — «до конца суток»), как в PaymentFilter. Sort — createdAt|rating
// ("" = createdAt), Dir — asc|desc ("" = desc). Пагинация 1-based: Limit по
// умолчанию 20, максимум 100; Page по умолчанию 1. Фильтры комбинируются
// через AND.
type AdminReviewFilter struct {
	CourseID string
	Rating   *int
	Q        string
	From, To time.Time
	Sort     string
	Dir      string
	Page     int
	Limit    int
}

// AdminReviewRow — строка глобального списка отзывов в форме контракта T83:
// отзыв + данные курса (title/slug) и ученика (email/имя).
type AdminReviewRow struct {
	ID             string
	CourseID       string
	CourseTitle    string
	CourseSlug     string
	StudentID      string
	StudentEmail   string
	StudentName    string
	Rating         int
	Body           string
	CreatedAt      time.Time
	TeacherReply   string
	TeacherReplyAt *time.Time
}

// normalizeAdminReviewSort сводит Sort к известному ключу: неизвестное или
// пустое значение — createdAt (дефолт контракта). Handler отвечает 400 на
// мусор; нормализация здесь — страховка для прямых вызовов хранилища.
func normalizeAdminReviewSort(s string) string {
	if s == "rating" {
		return "rating"
	}
	return "createdAt"
}

// normalizeAdminReviewDir сводит Dir к asc|desc: всё, кроме asc, — desc.
func normalizeAdminReviewDir(d string) string {
	if d == "asc" {
		return "asc"
	}
	return "desc"
}

// adminReviewSortCmp сравнивает строки по первичному ключу сортировки:
// <0 — a раньше b при возрастании, 0 — значения равны. Вторичный ключ —
// createdAt по убыванию, затем id по убыванию: порядок детерминирован и
// совпадает у in-memory и Postgres.
func adminReviewSortCmp(a, b *AdminReviewRow, sortKey string) int {
	if sortKey == "rating" {
		switch {
		case a.Rating < b.Rating:
			return -1
		case a.Rating > b.Rating:
			return 1
		}
		return 0
	}
	return cmpTime(a.CreatedAt, b.CreatedAt)
}

// sortAdminReviews упорядочивает отзывы по Sort/Dir с детерминированным
// вторичным ключом (createdAt desc, id desc).
func sortAdminReviews(rows []*AdminReviewRow, sortKey, dir string) {
	asc := normalizeAdminReviewDir(dir) == "asc"
	sort.Slice(rows, func(i, j int) bool {
		cmp := adminReviewSortCmp(rows[i], rows[j], sortKey)
		if cmp == 0 {
			if c := cmpTime(rows[i].CreatedAt, rows[j].CreatedAt); c != 0 {
				return c > 0
			}
			return rows[i].ID > rows[j].ID
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})
}

// adminReviewMatchesQ проверяет поиск T83 без учёта регистра: название
// курса, email/имя ученика и текст отзыва. q уже приведён к нижнему регистру
// и обрезан.
func adminReviewMatchesQ(row *AdminReviewRow, q string) bool {
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(row.CourseTitle), q) ||
		strings.Contains(strings.ToLower(row.StudentEmail), q) ||
		strings.Contains(strings.ToLower(row.StudentName), q) ||
		strings.Contains(strings.ToLower(row.Body), q)
}
