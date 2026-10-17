package repo

// T25 — SQL-реализации методов админ-консоли (см. admin_console.go:
// там описания, in-memory-варианты и объявления типов).
//
// Все агрегаты считаются SQL-ом (COUNT/SUM/AVG с индексами), таблицы
// в Go не выгружаются. Ошибки БД не глотаются там, где сигнатура позволяет
// вернуть ошибку (DeleteReview); методы без error логируют сбой и отдают
// пустой результат — так же, как исторические методы PostgresStore
// (например ListCoursesByStatus).

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AdminOverview собирает сводку одним SQL-запросом из скалярных подзапросов:
// пользователи по ролям, курсы по статусам, успешные платежи (суммы и за 30
// дней), записи на курсы и отзывы. Все COUNT приведены к int, суммы — к int
// (суммы копеек в INT не переполнятся на MVP-объёмах), средний рейтинг — к
// float8 с округлением до 1 знака.
func (s *PostgresStore) AdminOverview() AdminTotals {
	var t AdminTotals
	ctx, cancel := s.ctx()
	defer cancel()
	// UsersBlocked не сканируется: в схеме нет признака блокировки,
	// остаётся нулевым (см. комментарий к AdminTotals).
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*)::int FROM users),
		  (SELECT COUNT(*)::int FROM users WHERE role = 'student'),
		  (SELECT COUNT(*)::int FROM users WHERE role = 'teacher'),
		  (SELECT COUNT(*)::int FROM users WHERE role = 'admin'),
		  (SELECT COUNT(*)::int FROM users WHERE created_at >= now() - interval '7 days'),
		  (SELECT COUNT(*)::int FROM courses),
		  (SELECT COUNT(*)::int FROM courses WHERE status = 'draft'),
		  (SELECT COUNT(*)::int FROM courses WHERE status = 'review'),
		  (SELECT COUNT(*)::int FROM courses WHERE status = 'published'),
		  (SELECT COUNT(*)::int FROM courses WHERE status = 'archived'),
		  (SELECT COUNT(*)::int FROM payments WHERE status = 'succeeded'),
		  (SELECT COALESCE(SUM(amount_cents), 0)::int FROM payments WHERE status = 'succeeded'),
		  (SELECT COALESCE(SUM(platform_fee_cents), 0)::int FROM payments WHERE status = 'succeeded'),
		  (SELECT COALESCE(SUM(teacher_amount_cents), 0)::int FROM payments WHERE status = 'succeeded'),
		  (SELECT COALESCE(SUM(amount_cents), 0)::int FROM payments
		     WHERE status = 'succeeded' AND created_at >= now() - interval '30 days'),
		  (SELECT COUNT(*)::int FROM enrollments),
		  (SELECT COUNT(*)::int FROM enrollments WHERE status = 'active'),
		  (SELECT COUNT(*)::int FROM reviews),
		  (SELECT COALESCE(ROUND(AVG(rating)::numeric, 1), 0)::float8 FROM reviews)
	`).Scan(
		&t.UsersTotal, &t.UsersStudents, &t.UsersTeachers, &t.UsersAdmins, &t.UsersNew7,
		&t.CoursesTotal, &t.CoursesDraft, &t.CoursesReview, &t.CoursesPublished, &t.CoursesArchived,
		&t.PaymentsCount, &t.PaidCents, &t.PlatformFeeCents, &t.TeacherCents, &t.PaidLast30Cents,
		&t.EnrollmentsTotal, &t.EnrollmentsActive,
		&t.ReviewsTotal, &t.AvgRating,
	)
	if err != nil {
		log.Printf("postgres AdminOverview: %v", err)
		return AdminTotals{}
	}
	return t
}

// ListAllCourses возвращает курсы всех статусов, новые первыми; total — до
// пагинации. Фильтры status/q идут параметрами ($N) — без конкатенации
// пользовательского ввода в SQL; q — подстрока по title регистронезависимо.
func (s *PostgresStore) ListAllCourses(f CourseFilter) ([]*Course, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)

	where := ""
	args := make([]any, 0, 2)
	if f.Status != "" {
		args = append(args, string(f.Status))
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, q)
		where += fmt.Sprintf(" AND title ILIKE '%%'||$%d||'%%'", len(args))
	}

	ctx, cancel := s.ctx()
	defer cancel()

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM courses WHERE 1=1`+where, args...).Scan(&total); err != nil {
		log.Printf("postgres ListAllCourses count: %v", err)
		return nil, 0
	}

	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx, fmt.Sprintf(
		`SELECT %s FROM courses WHERE 1=1%s ORDER BY created_at DESC LIMIT %d OFFSET %d`,
		courseCols, where, limit, offset), args...)
	if err != nil {
		log.Printf("postgres ListAllCourses: %v", err)
		return nil, 0
	}
	defer rows.Close()

	items := make([]*Course, 0, limit)
	for rows.Next() {
		if c, err := scanCourse(rows.Scan); err == nil {
			items = append(items, c)
		}
	}
	return items, total
}

// DeleteReview удаляет отзыв и пересчитывает рейтинг курса (T25).
// ErrNotFound — отзыва нет (в том числе если id не UUID: чтобы не отдавать
// 500 на мусорном значении пути, оно трактуется как «не найдено», как и в
// in-memory-варианте).
func (s *PostgresStore) DeleteReview(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()

	var courseID string
	err := s.pool.QueryRow(ctx,
		`DELETE FROM reviews WHERE id=$1::uuid RETURNING course_id::text`, id).
		Scan(&courseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("не удалось удалить отзыв: %w", err)
	}
	s.RecalcCourseRating(courseID)
	return nil
}

// ListEnrollmentRows возвращает записи на курсы с данными ученика и курса,
// новые первыми; total — до пагинации. Прогресс (пройденные уроки) и размер
// программы считаются скалярными подзапросами на строку выборки — без N+1
// на уровне Go и без выгрузки таблиц.
func (s *PostgresStore) ListEnrollmentRows(f EnrollmentFilter) ([]EnrollmentRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)

	where := ""
	args := make([]any, 0, 2)
	if f.CourseID != "" {
		args = append(args, f.CourseID)
		where += fmt.Sprintf(" AND e.course_id=$%d::uuid", len(args))
	}
	if f.StudentID != "" {
		args = append(args, f.StudentID)
		where += fmt.Sprintf(" AND e.student_id=$%d::uuid", len(args))
	}

	ctx, cancel := s.ctx()
	defer cancel()

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM enrollments e WHERE 1=1`+where, args...).Scan(&total); err != nil {
		log.Printf("postgres ListEnrollmentRows count: %v", err)
		return nil, 0
	}

	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT e.id::text, e.student_id::text, e.course_id::text, e.status, e.created_at,
		       COALESCE(u.email, ''), COALESCE(u.display_name, ''), COALESCE(c.title, ''),
		       (SELECT COUNT(*)::int FROM lesson_progress lp
		          JOIN lessons l ON l.id = lp.lesson_id
		          JOIN modules m ON m.id = l.module_id
		         WHERE m.course_id = e.course_id
		           AND lp.student_id = e.student_id
		           AND lp.completed_at IS NOT NULL),
		       (SELECT COUNT(*)::int FROM lessons l
		          JOIN modules m ON m.id = l.module_id
		         WHERE m.course_id = e.course_id)
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		LEFT JOIN users u ON u.id = e.student_id
		WHERE 1=1%s
		ORDER BY e.created_at DESC LIMIT %d OFFSET %d`, where, limit, offset), args...)
	if err != nil {
		log.Printf("postgres ListEnrollmentRows: %v", err)
		return nil, 0
	}
	defer rows.Close()

	items := make([]EnrollmentRow, 0, limit)
	for rows.Next() {
		var row EnrollmentRow
		var status string
		if err := rows.Scan(&row.ID, &row.StudentID, &row.CourseID, &status, &row.CreatedAt,
			&row.StudentEmail, &row.StudentName, &row.CourseTitle,
			&row.Completed, &row.TotalLessons); err != nil {
			log.Printf("postgres ListEnrollmentRows scan: %v", err)
			return nil, 0
		}
		row.Status = EnrollmentStatus(status)
		items = append(items, row)
	}
	return items, total
}

// CourseContentCounts возвращает по каждому курсу модули, уроки и число
// активных учеников одним запросом (T25: без N+1 в списке курсов админки).
// студентов считаем по активным записям — как в in-memory-варианте.
func (s *PostgresStore) CourseContentCounts() map[string]ContentCounts {
	ctx, cancel := s.ctx()
	defer cancel()

	out := make(map[string]ContentCounts)
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text,
		       (SELECT COUNT(*)::int FROM modules m WHERE m.course_id = c.id),
		       (SELECT COUNT(*)::int FROM lessons l
		          JOIN modules m ON m.id = l.module_id
		         WHERE m.course_id = c.id),
		       (SELECT COUNT(*)::int FROM enrollments e
		         WHERE e.course_id = c.id AND e.status = 'active')
		FROM courses c`)
	if err != nil {
		log.Printf("postgres CourseContentCounts: %v", err)
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var cc ContentCounts
		if err := rows.Scan(&id, &cc.Modules, &cc.Lessons, &cc.Students); err == nil {
			out[id] = cc
		}
	}
	return out
}
