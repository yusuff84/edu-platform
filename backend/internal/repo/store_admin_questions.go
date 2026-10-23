package repo

// T85 — in-memory-реализация глобального списка Q&A для админки.
// См. admin_questions.go (типы и фильтр) и postgres_admin_questions.go
// (SQL-ветка). Данные читаются из тех же карт T71 (lessonQuestions/
// lessonAnswers/users/courses/lessons), отдельного индекса не заводим:
// объём MVP позволяет пройти вопросы один раз под RLock.

import (
	"sort"
	"strings"
)

// adminAnswerRowLocked собирает ответ с email/именем/ролью автора.
// Вызывающий держит s.mu (RLock достаточно).
func (s *Store) adminAnswerRowLocked(a *LessonAnswer) *AdminAnswerRow {
	row := &AdminAnswerRow{
		ID:        a.ID,
		AuthorID:  a.AuthorID,
		Body:      a.Body,
		CreatedAt: a.CreatedAt,
	}
	if u, ok := s.users[a.AuthorID]; ok {
		row.AuthorEmail = u.Email
		row.AuthorName = u.DisplayName
		row.AuthorRole = string(u.Role)
	}
	return row
}

// adminQuestionRowLocked собирает строку админки из вопроса T71: данные
// курса/урока/студента и ответы старыми первыми. Answers всегда не-nil.
// Вызывающий держит s.mu (RLock достаточно).
func (s *Store) adminQuestionRowLocked(q *LessonQuestion) *AdminQuestionRow {
	row := &AdminQuestionRow{
		ID:        q.ID,
		CourseID:  q.CourseID,
		LessonID:  q.LessonID,
		StudentID: q.StudentID,
		Body:      q.Body,
		CreatedAt: q.CreatedAt,
		Answers:   make([]*AdminAnswerRow, 0, len(s.lessonAnswerIDs[q.ID])),
	}
	if c, ok := s.courses[q.CourseID]; ok {
		row.CourseTitle = c.Title
		row.CourseSlug = c.Slug
	}
	if l, ok := s.lessons[q.LessonID]; ok {
		row.LessonTitle = l.Title
	}
	if u, ok := s.users[q.StudentID]; ok {
		row.StudentEmail = u.Email
		row.StudentName = u.DisplayName
	}
	for _, id := range s.lessonAnswerIDs[q.ID] {
		if a, ok := s.lessonAnswers[id]; ok {
			row.Answers = append(row.Answers, s.adminAnswerRowLocked(a))
		}
	}
	sort.Slice(row.Answers, func(i, j int) bool {
		if row.Answers[i].CreatedAt.Equal(row.Answers[j].CreatedAt) {
			return row.Answers[i].ID < row.Answers[j].ID
		}
		return row.Answers[i].CreatedAt.Before(row.Answers[j].CreatedAt)
	})
	return row
}

// adminQuestionMatchesLocked — поиск T85 без учёта регистра по вопросу,
// ответам, email/именам студента и авторов ответов, названиям курса и урока.
// q уже приведён к нижнему регистру и обрезан. Вызывающий держит s.mu.
func (s *Store) adminQuestionMatchesLocked(q *LessonQuestion, search string) bool {
	contains := func(v string) bool {
		return strings.Contains(strings.ToLower(v), search)
	}
	if contains(q.Body) {
		return true
	}
	if c, ok := s.courses[q.CourseID]; ok && contains(c.Title) {
		return true
	}
	if l, ok := s.lessons[q.LessonID]; ok && contains(l.Title) {
		return true
	}
	if u, ok := s.users[q.StudentID]; ok {
		if contains(u.Email) || contains(u.DisplayName) {
			return true
		}
	}
	for _, id := range s.lessonAnswerIDs[q.ID] {
		a, ok := s.lessonAnswers[id]
		if !ok {
			continue
		}
		if contains(a.Body) {
			return true
		}
		if u, ok := s.users[a.AuthorID]; ok {
			if contains(u.Email) || contains(u.DisplayName) {
				return true
			}
		}
	}
	return false
}

// ListAdminQuestions — глобальный список вопросов Q&A с фильтрами и поиском
// (T85). Новые вопросы первыми (createdAt desc, id desc), ответы внутри —
// старые первыми, total считается до пагинации. Никогда не возвращает
// nil-срез; у каждой строки answers — не-nil срез.
func (s *Store) ListAdminQuestions(f AdminQuestionFilter) ([]*AdminQuestionRow, int) {
	page, limit := clampPageLimit(f.Page, f.Limit)
	search := strings.ToLower(strings.TrimSpace(f.Q))

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*LessonQuestion, 0, len(s.lessonQuestions))
	for _, q := range s.lessonQuestions {
		if f.CourseID != "" && q.CourseID != f.CourseID {
			continue
		}
		if f.LessonID != "" && q.LessonID != f.LessonID {
			continue
		}
		if search != "" && !s.adminQuestionMatchesLocked(q, search) {
			continue
		}
		matched = append(matched, q)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items := make([]*AdminQuestionRow, 0, end-start)
	for _, q := range matched[start:end] {
		items = append(items, s.adminQuestionRowLocked(q))
	}
	return items, total
}
