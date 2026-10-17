package repo

// T23 — Postgres-реализации доработок кабинета ученика (in-memory зеркало —
// me_extra.go). Сортировка — в SQL; при сбое БД методы возвращают пустой
// (но не nil) результат, как и требует контракт StoreAPI.

// ListCompletedLessonIDs — id пройденных уроков курса в порядке программы:
// JOIN с modules/lessons и ORDER BY position модуля, затем позиции урока.
// Никогда не nil (пустой слайс — если ничего не пройдено).
func (s *PostgresStore) ListCompletedLessonIDs(studentID, courseID string) []string {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT l.id::text FROM lesson_progress lp
		 JOIN lessons l ON l.id = lp.lesson_id
		 JOIN modules m ON m.id = l.module_id
		 WHERE lp.student_id=$1::uuid AND m.course_id=$2::uuid
		   AND lp.completed_at IS NOT NULL
		 ORDER BY m.position ASC, l.position ASC`, studentID, courseID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// ListPaymentsByStudent — платежи ученика во всех статусах, новые первыми.
// Никогда не nil (пустой слайс — если платежей нет).
func (s *PostgresStore) ListPaymentsByStudent(studentID string) []*Payment {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE student_id=$1::uuid ORDER BY created_at DESC`,
		studentID)
	if err != nil {
		return []*Payment{}
	}
	defer rows.Close()
	out := make([]*Payment, 0)
	for rows.Next() {
		if p, err := scanPayment(rows.Scan); err == nil {
			out = append(out, p)
		}
	}
	return out
}
