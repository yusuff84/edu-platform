package repo

// SQL-реализации методов раздела «Пользователи» супер-админки (T24);
// семантика и in-memory-зеркала — в users_admin.go.

import (
	"fmt"
	"log"
	"strings"
)

// ListUsers — фильтрованный список пользователей для админки (T24).
// Q — подстрока по email/displayName (ILIKE), Role/Status — точное
// совпадение, "" — любой; новые первыми (created_at DESC).
func (s *PostgresStore) ListUsers(f UserFilter) []*User {
	// Динамический WHERE: значения идут параметрами через $N (без
	// конкатенации пользовательского ввода в SQL).
	where := ""
	args := make([]any, 0, 3)
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, q)
		where += fmt.Sprintf(" AND (email ILIKE '%%'||$%d||'%%' OR COALESCE(display_name,'') ILIKE '%%'||$%d||'%%')", len(args), len(args))
	}
	if f.Role != "" {
		args = append(args, string(f.Role))
		where += fmt.Sprintf(" AND role=$%d", len(args))
	}
	if f.Status != "" {
		args = append(args, string(f.Status))
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}

	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, email, password_hash, role, COALESCE(display_name,''), created_at, status
		 FROM users WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		log.Printf("postgres ListUsers: %v", err)
		return nil
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.DisplayName,
			&u.CreatedAt, &u.Status); err == nil {
			out = append(out, u)
		}
	}
	return out
}

// SetUserStatus активен/заблокирован; false — пользователя нет или БД
// недоступна (T24). Приходит уже провалидированным из handler'а.
func (s *PostgresStore) SetUserStatus(id string, status UserStatus) bool {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE users SET status=$2 WHERE id=$1::uuid`, id, string(status))
	if err != nil {
		log.Printf("postgres SetUserStatus: %v", err)
		return false
	}
	return tag.RowsAffected() > 0
}

// DeleteUser удаляет пользователя (T24); ErrNotFound — нет такого.
// В одной транзакции явно чистятся сессии, прогресс, отзывы и записи на
// курсы — зеркало in-memory Store (в 001_init у части таблиц стоит
// ON DELETE CASCADE, но опираться на него одним DELETE нельзя: поведение
// должно совпадать у обеих реализаций). Курсы и платежи НЕ удаляются —
// handler обязан отказать (409) при их наличии (см. UserDependencies).
func (s *PostgresStore) DeleteUser(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("удаление пользователя: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // откат после Commit безвреден
	for _, q := range []string{
		`DELETE FROM sessions WHERE user_id=$1::uuid`,
		`DELETE FROM lesson_progress WHERE student_id=$1::uuid`,
		`DELETE FROM reviews WHERE student_id=$1::uuid`,
		`DELETE FROM enrollments WHERE student_id=$1::uuid`,
	} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return fmt.Errorf("удаление пользователя: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id=$1::uuid`, id)
	if err != nil {
		return fmt.Errorf("удаление пользователя: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("удаление пользователя: %w", err)
	}
	return nil
}

// UserDependencies — счётчики связей пользователя (T24): курсы, платежи,
// записи на курсы. Ошибка чтения логируется: при недоступной БД нули
// безопасны — последующий DeleteUser в той же ситуации тоже не пройдёт.
func (s *PostgresStore) UserDependencies(id string) UserDeps {
	ctx, cancel := s.ctx()
	defer cancel()
	var deps UserDeps
	err := s.pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM courses WHERE teacher_id=$1::uuid),
		        (SELECT COUNT(*) FROM payments WHERE student_id=$1::uuid),
		        (SELECT COUNT(*) FROM enrollments WHERE student_id=$1::uuid)`, id).
		Scan(&deps.Courses, &deps.Payments, &deps.Enrollments)
	if err != nil {
		log.Printf("postgres UserDependencies: %v", err)
	}
	return deps
}

// CountUsersByRole — сколько пользователей с ролью (T24: защита последнего
// администратора). При ошибке чтения возвращает 0 — handler трактует это
// как «последний» и отказывает (409, fail-safe).
func (s *PostgresStore) CountUsersByRole(role Role) int {
	ctx, cancel := s.ctx()
	defer cancel()
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE role=$1`, string(role)).Scan(&n); err != nil {
		log.Printf("postgres CountUsersByRole: %v", err)
		return 0
	}
	return n
}
