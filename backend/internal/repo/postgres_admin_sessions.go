package repo

// SQL-реализации методов управления сессиями пользователя в супер-админке
// (T93); семантика и in-memory-зеркала — в store_admin_sessions.go.

import "log"

// ListSessionsByUser — активные (не истёкшие) сессии пользователя, новые
// первыми (T93): `expires_at > now()`, `ORDER BY created_at DESC`.
// Дополнительный ключ `id DESC` детерминирует порядок сессий, созданных в
// одну микросекунду, и совпадает с сортировкой in-memory реализации.
//
// Срез никогда не nil: при ошибке чтения возвращается пустой срез (хендлер
// отдаёт `items: []`; недоступность БД не должна превращать ответ в `null`).
func (s *PostgresStore) ListSessionsByUser(userID string) []*Session {
	out := make([]*Session, 0)
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, user_id::text, created_at, expires_at
		 FROM sessions
		 WHERE user_id=$1::uuid AND expires_at > now()
		 ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		log.Printf("postgres ListSessionsByUser: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		sess := &Session{}
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt); err != nil {
			log.Printf("postgres ListSessionsByUser: scan: %v", err)
			continue
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		log.Printf("postgres ListSessionsByUser: %v", err)
	}
	return out
}

// DeleteSessionsByUser удаляет все сессии пользователя (включая истёкшие) и
// возвращает их количество (T93). Отсутствие сессий — 0, не ошибка.
func (s *PostgresStore) DeleteSessionsByUser(userID string) int {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1::uuid`, userID)
	if err != nil {
		log.Printf("postgres DeleteSessionsByUser: %v", err)
		return 0
	}
	return int(tag.RowsAffected())
}
