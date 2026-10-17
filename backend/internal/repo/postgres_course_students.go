package repo

// T22 — ученики курса и статистика для учителя: методы PostgresStore.
//
// N+1 не допускается: страница «Ученики курса» собирается тремя запросами
// на курс —
//  1) enrollments по course_id (новые первыми);
//  2) lesson_progress + lessons + modules по course_id — агрегат по ученикам;
//  3) payments по course_id.
//
// Ошибки БД логируются и дают пустой результат: сигнатуры StoreAPI (T22) не
// возвращают error (та же манера, что у большинства методов postgres_store.go);
// статистика/список учеников при сбое не должны валить запрос 500-й.

import (
	"log"
	"time"
)

// ListEnrollmentsByCourse — все записи на курс, новые первыми (T22).
// Записи во всех статусах: учитель должен видеть и отчисления.
func (s *PostgresStore) ListEnrollmentsByCourse(courseID string) []*Enrollment {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+enrollCols+` FROM enrollments
		 WHERE course_id=$1::uuid ORDER BY created_at DESC`, courseID)
	if err != nil {
		log.Printf("postgres ListEnrollmentsByCourse: %v", err)
		return nil
	}
	defer rows.Close()
	out := make([]*Enrollment, 0)
	for rows.Next() {
		e, err := scanEnrollment(rows.Scan)
		if err != nil {
			log.Printf("postgres ListEnrollmentsByCourse scan: %v", err)
			continue
		}
		out = append(out, e)
	}
	return out
}

// StudentProgress — карта studentID → прогресс по курсу одним запросом (T22):
// агрегат по lesson_progress с JOIN lessons+modules (только уроки курса) и
// JOIN enrollments (ключи — только записанные на курс, в любом статусе).
// completed_at может быть NULL (урок начат, но не завершён): COUNT считает
// только заполненные значения, MAX игнорирует NULL.
func (s *PostgresStore) StudentProgress(courseID string) map[string]StudentProgress {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT lp.student_id::text,
		        COUNT(lp.completed_at)::int,
		        MAX(lp.completed_at)
		 FROM lesson_progress lp
		 JOIN lessons l ON l.id = lp.lesson_id
		 JOIN modules m ON m.id = l.module_id
		 JOIN enrollments e ON e.student_id = lp.student_id AND e.course_id = m.course_id
		 WHERE m.course_id=$1::uuid
		 GROUP BY lp.student_id`, courseID)
	if err != nil {
		log.Printf("postgres StudentProgress: %v", err)
		return nil
	}
	defer rows.Close()
	out := make(map[string]StudentProgress)
	for rows.Next() {
		var studentID string
		var sp StudentProgress
		var last *time.Time
		if err := rows.Scan(&studentID, &sp.Completed, &last); err != nil {
			log.Printf("postgres StudentProgress scan: %v", err)
			continue
		}
		if last != nil {
			sp.LastActivity = *last
		}
		out[studentID] = sp
	}
	return out
}

// ListPaymentsByCourse — платежи по курсу (T22), новые первыми; фильтрацию по
// статусу (выручка — только succeeded) делает handler.
func (s *PostgresStore) ListPaymentsByCourse(courseID string) []*Payment {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+paymentCols+` FROM payments
		 WHERE course_id=$1::uuid ORDER BY created_at DESC`, courseID)
	if err != nil {
		log.Printf("postgres ListPaymentsByCourse: %v", err)
		return nil
	}
	defer rows.Close()
	out := make([]*Payment, 0)
	for rows.Next() {
		p, err := scanPayment(rows.Scan)
		if err != nil {
			log.Printf("postgres ListPaymentsByCourse scan: %v", err)
			continue
		}
		out = append(out, p)
	}
	return out
}
