package repo

// T22 — ученики курса и статистика для учителя: методы in-memory Store.
// Сигнатуры и семантика совпадают с Postgres-реализацией
// (postgres_course_students.go); контракт методов — StoreAPI в store_iface.go.

import "sort"

// ListEnrollmentsByCourse — все записи на курс, новые первыми (T22).
// Записи во всех статусах: учитель должен видеть и отчисления (refunded,
// revoked), статус отдаётся наружу полем.
func (s *Store) ListEnrollmentsByCourse(courseID string) []*Enrollment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Enrollment, 0)
	for _, e := range s.enrollments {
		if e.CourseID == courseID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// StudentProgress — карта studentID → прогресс по курсу (T22): один проход по
// картам progress/lessons/modules/enrollments, без отдельных запросов на
// ученика. Completed — только уроки с заполненным CompletedAt; LastActivity —
// максимум CompletedAt по урокам курса (нулевое время — не начинал).
// Ключи — только ученики, у которых есть запись на курс.
func (s *Store) StudentProgress(courseID string) map[string]StudentProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Уроки курса: module -> course, lesson -> module.
	modIDs := make(map[string]bool)
	for _, m := range s.modules {
		if m.CourseID == courseID {
			modIDs[m.ID] = true
		}
	}
	lessonIDs := make(map[string]bool)
	for _, l := range s.lessons {
		if modIDs[l.ModuleID] {
			lessonIDs[l.ID] = true
		}
	}
	// Записанные на курс — в любом статусе (active/refunded/revoked): прогресс
	// отчисленного ученика учителю тоже виден.
	enrolled := make(map[string]bool)
	for _, e := range s.enrollments {
		if e.CourseID == courseID {
			enrolled[e.StudentID] = true
		}
	}

	out := make(map[string]StudentProgress)
	for _, p := range s.progress {
		if !enrolled[p.StudentID] || !lessonIDs[p.LessonID] {
			continue
		}
		sp := out[p.StudentID]
		if !p.CompletedAt.IsZero() {
			sp.Completed++
			if p.CompletedAt.After(sp.LastActivity) {
				sp.LastActivity = p.CompletedAt
			}
		}
		out[p.StudentID] = sp
	}
	return out
}

// ListPaymentsByCourse — платежи по курсу (T22), новые первыми; фильтрацию по
// статусу (выручка — только succeeded) делает handler: репозиторий отдаёт
// сырой список, чтобы не плодить методы под каждое представление.
func (s *Store) ListPaymentsByCourse(courseID string) []*Payment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Payment, 0)
	for _, p := range s.payments {
		if p.CourseID == courseID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
