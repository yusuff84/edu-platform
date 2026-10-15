package repo

// SQL-реализации частичных обновлений (см. update.go — там in-memory
// и комментарии к семантике PATCH).

import (
	"strconv"
	"strings"
)

// UpdateCourse применяет частичное обновление курса динамическим SET
// (только переданные поля); false — курса нет или БД недоступна.
func (s *PostgresStore) UpdateCourse(id string, u CourseUpdate) bool {
	sets := make([]string, 0, 5)
	args := make([]any, 0, 5)
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, col+"=$"+strconv.Itoa(len(args)))
	}
	if u.Title != nil {
		add("title", *u.Title)
	}
	if u.Description != nil {
		add("description", *u.Description)
	}
	if u.Slug != nil {
		add("slug", *u.Slug)
	}
	if u.PriceCents != nil {
		add("price_cents", *u.PriceCents)
	}
	sets = append(sets, "updated_at=now()")
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		"UPDATE courses SET "+strings.Join(sets, ", ")+" WHERE id=$1::uuid",
		append([]any{id}, args...)...)
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

// UpdateModuleTitle переименовывает модуль; false — модуля нет или БД недоступна.
func (s *PostgresStore) UpdateModuleTitle(id, title string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE modules SET title=$2 WHERE id=$1::uuid`, id, title)
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

// SetUserDisplayName задаёт отображаемое имя пользователя (OAuth-профиль).
func (s *PostgresStore) SetUserDisplayName(id, name string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE users SET display_name=$2 WHERE id=$1::uuid`, id, name)
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}
