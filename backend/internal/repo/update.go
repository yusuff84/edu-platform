package repo

// Частичные обновления записей (T13: бэклог T06 — редактирование курсов
// и модулей, вход через OAuth — displayName из профиля Яндекса).
//
// Методы вынесены в отдельный файл, чтобы не раздувать store.go и
// postgres_store.go и не создавать конфликты при слиянии волн.

import "time"

// CourseUpdate — частичное обновление карточки курса.
// nil-поля не меняются (семантика PATCH).
type CourseUpdate struct {
	Title       *string
	Description *string
	Slug        *string
	PriceCents  *int
	Category    *string // T65: nil — категория не меняется
}

// UpdateCourse применяет частичное обновление курса; false — курса нет.
func (s *Store) UpdateCourse(id string, u CourseUpdate) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return false
	}
	if u.Title != nil {
		c.Title = *u.Title
	}
	if u.Description != nil {
		c.Description = *u.Description
	}
	if u.Slug != nil {
		c.Slug = *u.Slug
	}
	if u.PriceCents != nil {
		c.PriceCents = *u.PriceCents
	}
	if u.Category != nil {
		c.Category = *u.Category
	}
	c.UpdatedAt = time.Now().UTC()
	return true
}

// UpdateModuleTitle переименовывает модуль; false — модуля нет.
func (s *Store) UpdateModuleTitle(id, title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.modules[id]
	if !ok {
		return false
	}
	m.Title = title
	return true
}

// SetUserDisplayName задаёт отображаемое имя пользователя; false — нет юзера.
func (s *Store) SetUserDisplayName(id, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return false
	}
	u.DisplayName = name
	return true
}
