package repo

// SQL-реализации частичных обновлений (см. update.go — там in-memory
// и комментарии к семантике PATCH).

import (
	"strconv"
	"strings"
)

// UpdateCourse применяет частичное обновление курса динамическим SET
// (только переданные поля); false — курса нет или БД недоступна.
//
// Нумерация плейсхолдеров: $1 — id курса в WHERE, поля SET получают $2, $3…
// (args заводится с id). Регресс qa-p4: если начинать SET с $1, запрос
// получает SET title=$1 … WHERE id=$1::uuid, а в Exec уходит на один
// параметр больше, чем есть в запросе — Postgres отвечает ошибкой bind и
// PATCH любой карточки курса на Postgres превращается в 500. В in-memory
// ветке (update.go) баг не проявляется, поэтому ловится только здесь.
//
// cover_path в SET НЕ входит принципиально (T54): PATCH карточки не обязан
// знать о файле обложки, путь меняет только SetCourseCover ниже.
func (s *PostgresStore) UpdateCourse(id string, u CourseUpdate) bool {
	sets := make([]string, 0, 6)
	args := []any{id}
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
	if u.Category != nil {
		add("category", *u.Category)
	}
	sets = append(sets, "updated_at=now()")
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		"UPDATE courses SET "+strings.Join(sets, ", ")+" WHERE id=$1::uuid",
		args...)
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

// SetCourseCover записывает путь обложки курса (T54): cover != nil — новый
// путь «/uploads/covers/<файл>», nil — сброс (cover_path=NULL). false —
// курса нет или БД недоступна (существование курса handler уже проверил
// через GetCourseByID, поэтому сюда доходят только реальные ошибки записи).
// updated_at двигаем, как у любой другой правки курса.
func (s *PostgresStore) SetCourseCover(id string, cover *string) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE courses SET cover_path=$2, updated_at=now() WHERE id=$1::uuid`,
		id, cover)
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
