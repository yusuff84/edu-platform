package repo

// T73 — Postgres-реализация избранных курсов (in-memory зеркало —
// favorites.go). Схема — 013_favorites.sql: PK (user_id, course_id),
// FK на users/courses с ON DELETE CASCADE, индекс (user_id, created_at DESC).
// В избранное можно добавить любой существующий курс; фильтр published
// применяется при чтении списка (контракт GET /me/favorites).

import (
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgconn"
)

// isInvalidUUID — ошибка приведения текста к uuid (22P02). Для контракта
// избранного это «такого курса нет»: POST/DELETE не должны отвечать 500 на
// мусорный id в URL.
func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

// AddFavorite идемпотентно добавляет курс в избранное: конфликт PK
// (user_id, course_id) игнорируется. ErrNotFound — пользователя или курса
// нет (нарушение FK) либо id не является UUID.
func (s *PostgresStore) AddFavorite(userID, courseID string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO favorites (user_id, course_id)
		 VALUES ($1::uuid,$2::uuid)
		 ON CONFLICT (user_id, course_id) DO NOTHING`,
		userID, courseID)
	if err != nil {
		if isForeignKeyViolation(err) || isInvalidUUID(err) {
			return ErrNotFound
		}
		return fmt.Errorf("не удалось добавить в избранное: %w", err)
	}
	return nil
}

// RemoveFavorite идемпотентно удаляет курс из избранного; отсутствие строки
// и мусорный id курса — успех (удалять нечего).
func (s *PostgresStore) RemoveFavorite(userID, courseID string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id=$1::uuid AND course_id=$2::uuid`,
		userID, courseID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil
		}
		return fmt.Errorf("не удалось удалить из избранного: %w", err)
	}
	return nil
}

// ListFavoriteCoursesByUser — published-курсы избранного, добавленные позже
// первыми (при равном created_at — по id курса по убыванию, чтобы порядок
// был детерминированным). Никогда не nil.
func (s *PostgresStore) ListFavoriteCoursesByUser(userID string) []*Course {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+courseColsJoined+`
		 FROM favorites f
		 JOIN courses c ON c.id = f.course_id
		 WHERE f.user_id = $1::uuid AND c.status = 'published'
		 ORDER BY f.created_at DESC, c.id DESC`, userID)
	if err != nil {
		log.Printf("postgres ListFavoriteCoursesByUser: %v", err)
		return []*Course{}
	}
	defer rows.Close()
	out := make([]*Course, 0)
	for rows.Next() {
		if c, err := scanCourse(rows.Scan); err == nil {
			out = append(out, c)
		}
	}
	return out
}
