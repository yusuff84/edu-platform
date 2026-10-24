package repo

// T85 — SQL-реализация глобального списка Q&A для админки.
// Типы и фильтр — admin_questions.go, in-memory-ветка —
// store_admin_questions.go. Запросы читают те же таблицы T71, но
// джойнят курс/урок/студента и авторов ответов, чтобы отдать контрактные
// поля courseTitle/courseSlug/studentEmail/authorEmail одним запросом.

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// adminQuestionFrom — общий FROM для COUNT и страницы: курс и урок нужны
// и в поиске по названиям, студент — для email/имени.
const adminQuestionFrom = `FROM lesson_questions q
	JOIN courses c ON c.id = q.course_id
	JOIN lessons l ON l.id = q.lesson_id
	JOIN users u ON u.id = q.student_id`

// adminQuestionWhere собирает WHERE для T85 по фильтрам. Пользовательский
// ввод передаётся только параметрами ($N). Поиск без учёта регистра идёт по
// вопросу, ответам, email/именам студента и авторов ответов, названиям курса
// и урока — как в in-memory-ветке. Возвращает условия без ведущего WHERE.
func adminQuestionWhere(f AdminQuestionFilter) (string, []any) {
	where := "1=1"
	args := make([]any, 0, 4)

	if id := strings.TrimSpace(f.CourseID); id != "" {
		args = append(args, id)
		where += fmt.Sprintf(" AND q.course_id=$%d::uuid", len(args))
	}
	if id := strings.TrimSpace(f.LessonID); id != "" {
		args = append(args, id)
		where += fmt.Sprintf(" AND q.lesson_id=$%d::uuid", len(args))
	}
	if search := strings.ToLower(strings.TrimSpace(f.Q)); search != "" {
		args = append(args, "%"+search+"%")
		n := len(args)
		where += fmt.Sprintf(` AND (
			lower(q.body) LIKE $%d
			OR lower(c.title) LIKE $%d
			OR lower(l.title) LIKE $%d
			OR lower(u.email) LIKE $%d
			OR lower(COALESCE(u.display_name,'')) LIKE $%d
			OR EXISTS (
				SELECT 1 FROM lesson_answers a
				JOIN users au ON au.id = a.author_id
				WHERE a.question_id = q.id AND (
					lower(a.body) LIKE $%d
					OR lower(au.email) LIKE $%d
					OR lower(COALESCE(au.display_name,'')) LIKE $%d
				)
			)
		)`, n, n, n, n, n, n, n, n)
	}
	return where, args
}

// loadAdminAnswers читает ответы для страницы вопросов одним запросом (без
// N+1) и раскладывает по questionId; внутри — старые первыми (createdAt asc,
// id asc — тот же детерминированный порядок, что в in-memory).
func (s *PostgresStore) loadAdminAnswers(ctx context.Context, questionIDs []string) map[string][]*AdminAnswerRow {
	out := make(map[string][]*AdminAnswerRow, len(questionIDs))
	if len(questionIDs) == 0 {
		return out
	}
	ph := make([]string, 0, len(questionIDs))
	args := make([]any, 0, len(questionIDs))
	for i, id := range questionIDs {
		ph = append(ph, fmt.Sprintf("$%d::uuid", i+1))
		args = append(args, id)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT a.id::text, a.question_id::text, a.author_id::text,
		        COALESCE(au.email,''), COALESCE(au.display_name,''), au.role::text,
		        a.body, a.created_at
		 FROM lesson_answers a
		 JOIN users au ON au.id = a.author_id
		 WHERE a.question_id IN (`+strings.Join(ph, ",")+`)
		 ORDER BY a.created_at ASC, a.id ASC`, args...)
	if err != nil {
		log.Printf("postgres loadAdminAnswers: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		a := &AdminAnswerRow{}
		var questionID string
		if err := rows.Scan(&a.ID, &questionID, &a.AuthorID,
			&a.AuthorEmail, &a.AuthorName, &a.AuthorRole,
			&a.Body, &a.CreatedAt); err != nil {
			log.Printf("postgres loadAdminAnswers scan: %v", err)
			continue
		}
		out[questionID] = append(out[questionID], a)
	}
	return out
}

// ListAdminQuestions — глобальный список вопросов Q&A с фильтрами и поиском
// (T85). Новые вопросы первыми (created_at desc, id desc), total — до
// пагинации, ответы внутри — старые первыми. Никогда не возвращает nil-срез.
// Некорректные UUID в фильтрах трактуются как пустая выборка: handler
// валидирует их раньше и отвечает 400, но прямой вызов хранилища не должен
// ронять SQL приведением к uuid.
func (s *PostgresStore) ListAdminQuestions(f AdminQuestionFilter) ([]*AdminQuestionRow, int) {
	empty := []*AdminQuestionRow{}
	if id := strings.TrimSpace(f.CourseID); id != "" && !validUUID(id) {
		return empty, 0
	}
	if id := strings.TrimSpace(f.LessonID); id != "" && !validUUID(id) {
		return empty, 0
	}
	page, limit := clampPageLimit(f.Page, f.Limit)
	where, args := adminQuestionWhere(f)

	ctx, cancel := s.ctx()
	defer cancel()

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int `+adminQuestionFrom+` WHERE `+where, args...).Scan(&total); err != nil {
		log.Printf("postgres ListAdminQuestions count: %v", err)
		return empty, 0
	}

	offset := (page - 1) * limit
	query := fmt.Sprintf(
		`SELECT q.id::text, q.course_id::text, c.title, c.slug,
		        q.lesson_id::text, l.title,
		        q.student_id::text, COALESCE(u.email,''), COALESCE(u.display_name,''),
		        q.body, q.created_at
		 %s
		 WHERE %s
		 ORDER BY q.created_at DESC, q.id DESC
		 LIMIT $%d OFFSET $%d`,
		adminQuestionFrom, where, len(args)+1, len(args)+2)
	args = append(args, limit, offset)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("postgres ListAdminQuestions: %v", err)
		return empty, 0
	}
	defer rows.Close()

	items := make([]*AdminQuestionRow, 0, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		row := &AdminQuestionRow{Answers: []*AdminAnswerRow{}}
		if err := rows.Scan(&row.ID, &row.CourseID, &row.CourseTitle, &row.CourseSlug,
			&row.LessonID, &row.LessonTitle,
			&row.StudentID, &row.StudentEmail, &row.StudentName,
			&row.Body, &row.CreatedAt); err != nil {
			log.Printf("postgres ListAdminQuestions scan: %v", err)
			continue
		}
		items = append(items, row)
		ids = append(ids, row.ID)
	}
	answers := s.loadAdminAnswers(ctx, ids)
	for _, q := range items {
		if list, ok := answers[q.ID]; ok {
			q.Answers = list
		}
	}
	return items, total
}
