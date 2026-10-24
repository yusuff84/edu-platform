package repo

// T83 — SQL-реализация глобального списка отзывов для админки (см.
// admin_reviews.go: типы, фильтр и общие помощники; store_admin_reviews.go:
// in-memory-вариант).
//
// total считается отдельным COUNT по тому же отфильтрованному набору, items —
// страница после LIMIT/OFFSET. Пользовательский ввод передаётся только
// параметрами $N (без конкатенации в SQL). JOIN users — LEFT: ученик без
// строки в users не должен терять отзыв; JOIN courses — по FK отзыва, строка
// ровно одна (c.id — первичный ключ).

import (
	"fmt"
	"log"
	"strings"
)

// ListAdminReviews возвращает страницу глобального списка отзывов (T83),
// total — до пагинации, items — никогда не nil. Ошибки БД логируются и
// приводят к пустому результату: сигнатура StoreAPI не отдаёт error.
func (s *PostgresStore) ListAdminReviews(f AdminReviewFilter) ([]*AdminReviewRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)

	where := ""
	args := make([]any, 0, 6)
	if f.CourseID != "" {
		// Мусорный UUID трактуем как «ничего не найдено», а не 500 из БД.
		if !validUUID(f.CourseID) {
			return make([]*AdminReviewRow, 0), 0
		}
		args = append(args, f.CourseID)
		where += fmt.Sprintf(" AND r.course_id=$%d::uuid", len(args))
	}
	if f.Rating != nil {
		args = append(args, *f.Rating)
		where += fmt.Sprintf(" AND r.rating=$%d", len(args))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where += fmt.Sprintf(
			` AND (lower(c.title) LIKE $%d
			   OR lower(COALESCE(u.email,'')) LIKE $%d
			   OR lower(COALESCE(u.display_name,'')) LIKE $%d
			   OR lower(COALESCE(r.body,'')) LIKE $%d)`,
			len(args), len(args), len(args), len(args))
	}
	// Границы периода включительно: from — «с начала суток», to — «до конца
	// суток» (created_at < следующих суток после to).
	if !f.From.IsZero() {
		args = append(args, f.From)
		where += fmt.Sprintf(" AND r.created_at >= $%d", len(args))
	}
	if !f.To.IsZero() {
		args = append(args, f.To.AddDate(0, 0, 1))
		where += fmt.Sprintf(" AND r.created_at < $%d", len(args))
	}

	orderBy := "r.created_at"
	if normalizeAdminReviewSort(f.Sort) == "rating" {
		orderBy = "r.rating"
	}
	dir := "DESC"
	if normalizeAdminReviewDir(f.Dir) == "asc" {
		dir = "ASC"
	}

	ctx, cancel := s.ctx()
	defer cancel()

	base := ` FROM reviews r
		JOIN courses c ON c.id = r.course_id
		LEFT JOIN users u ON u.id = r.student_id
		WHERE 1=1` + where

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int`+base, args...).Scan(&total); err != nil {
		log.Printf("postgres ListAdminReviews count: %v", err)
		return make([]*AdminReviewRow, 0), 0
	}

	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT r.id::text, r.course_id::text, c.title, c.slug,
		       r.student_id::text, COALESCE(u.email,''), COALESCE(u.display_name,''),
		       r.rating, COALESCE(r.body,''), r.created_at,
		       r.teacher_reply, r.teacher_reply_at`+base+`
		ORDER BY %s %s, r.created_at DESC, r.id DESC
		LIMIT %d OFFSET %d`, orderBy, dir, limit, offset), args...)
	if err != nil {
		log.Printf("postgres ListAdminReviews: %v", err)
		return make([]*AdminReviewRow, 0), 0
	}
	defer rows.Close()

	items := make([]*AdminReviewRow, 0, limit)
	for rows.Next() {
		row := &AdminReviewRow{}
		if err := rows.Scan(
			&row.ID, &row.CourseID, &row.CourseTitle, &row.CourseSlug,
			&row.StudentID, &row.StudentEmail, &row.StudentName,
			&row.Rating, &row.Body, &row.CreatedAt,
			&row.TeacherReply, &row.TeacherReplyAt,
		); err != nil {
			log.Printf("postgres ListAdminReviews scan: %v", err)
			return make([]*AdminReviewRow, 0), 0
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		log.Printf("postgres ListAdminReviews rows: %v", err)
	}
	return items, total
}
