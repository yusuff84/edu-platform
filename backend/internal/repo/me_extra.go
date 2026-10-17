package repo

// T23 — доработки кабинета ученика: список пройденных уроков по курсу и
// история платежей. Методы вынесены в отдельный файл, чтобы не раздувать
// store.go и не создавать конфликты при слиянии волн (та же схема, что у
// update.go от T13).

import "sort"

// ListCompletedLessonIDs — id пройденных уроков курса в порядке программы
// (модуль за модулем, позиция внутри модуля). T23: в прогресс-эндпоинте
// важно не количество, а сами id в стабильном порядке.
// Никогда не nil: если ничего не пройдено — пустой слайс.
func (s *Store) ListCompletedLessonIDs(studentID, courseID string) []string {
	lessons := s.ListLessonsByCourse(courseID)
	out := make([]string, 0, len(lessons))
	for _, l := range lessons {
		if s.IsLessonCompleted(studentID, l.ID) {
			out = append(out, l.ID)
		}
	}
	return out
}

// ListPaymentsByStudent — платежи ученика во всех статусах, новые первыми
// (T23: ученик должен видеть и pending, и failed). Никогда не nil.
func (s *Store) ListPaymentsByStudent(studentID string) []*Payment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Payment, 0)
	for _, p := range s.payments {
		if p.StudentID == studentID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}
