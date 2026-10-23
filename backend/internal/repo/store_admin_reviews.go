package repo

// T83 — in-memory реализация глобального списка отзывов для админки.
// Типы и общие помощники — в admin_reviews.go, SQL — в
// postgres_admin_reviews.go.

import (
	"strings"
)

// ListAdminReviews возвращает отзывы всех курсов с фильтрами/поиском/
// сортировкой и пагинацией (T83); total — до пагинации, items — никогда не
// nil. Данные курса и ученика подтягиваются из карт под тем же RLock:
// вложенные вызовы методов Store взяли бы лок повторно. Наружу отдаются
// снимки — отзыв изменяемый (ответ преподавателя, T69).
func (s *Store) ListAdminReviews(f AdminReviewFilter) ([]*AdminReviewRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)
	sortKey := normalizeAdminReviewSort(f.Sort)
	q := strings.ToLower(strings.TrimSpace(f.Q))

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*AdminReviewRow, 0, len(s.reviews))
	for _, rv := range s.reviews {
		if f.CourseID != "" && rv.CourseID != f.CourseID {
			continue
		}
		if f.Rating != nil && rv.Rating != *f.Rating {
			continue
		}
		// Границы периода включительно; To — «до конца суток» (та же
		// семантика, что у ListPaymentsJournal и ListAllCourses).
		if !f.From.IsZero() && rv.CreatedAt.Before(f.From) {
			continue
		}
		if !f.To.IsZero() && !rv.CreatedAt.Before(f.To.AddDate(0, 0, 1)) {
			continue
		}

		row := &AdminReviewRow{
			ID: rv.ID, CourseID: rv.CourseID, StudentID: rv.StudentID,
			Rating: rv.Rating, Body: rv.Body, CreatedAt: rv.CreatedAt,
			TeacherReply: rv.TeacherReply, TeacherReplyAt: rv.TeacherReplyAt,
		}
		if c, ok := s.courses[rv.CourseID]; ok {
			row.CourseTitle = c.Title
			row.CourseSlug = c.Slug
		}
		if u, ok := s.users[rv.StudentID]; ok {
			row.StudentEmail = u.Email
			row.StudentName = u.DisplayName
		}
		if !adminReviewMatchesQ(row, q) {
			continue
		}
		matched = append(matched, row)
	}

	sortAdminReviews(matched, sortKey, f.Dir)

	total := len(matched)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items := make([]*AdminReviewRow, 0, end-start)
	items = append(items, matched[start:end]...)
	return items, total
}
