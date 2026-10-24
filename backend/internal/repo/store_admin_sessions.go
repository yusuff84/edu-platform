package repo

// In-memory реализация методов управления сессиями пользователя в
// супер-админке (T93). Вынесена в отдельный файл, как users_admin.go у T24,
// чтобы не раздувать store.go.

import (
	"sort"
	"time"
)

// ListSessionsByUser — активные (не истёкшие) сессии пользователя, новые
// первыми (T93). Истёкшие записи в ответ не попадают; сам срез никогда не
// nil — пустой список сериализуется хендлером как `[]`, не `null`.
//
// Время создания в in-memory режиме может совпадать (системные часы Windows
// тикают раз в ~15 мс), поэтому при равенстве CreatedAt порядок
// детерминируется id по убыванию — так же, как `ORDER BY created_at DESC,
// id DESC` в Postgres.
func (s *Store) ListSessionsByUser(userID string) []*Session {
	now := time.Now().UTC()
	s.mu.RLock()
	out := make([]*Session, 0)
	for _, sess := range s.sessions {
		if sess.UserID != userID || !sess.ExpiresAt.After(now) {
			continue
		}
		out = append(out, sess)
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// DeleteSessionsByUser удаляет все сессии пользователя (включая истёкшие,
// если они ещё лежат в карте) и возвращает их количество (T93). Отсутствие
// сессий — не ошибка: возвращается 0.
func (s *Store) DeleteSessionsByUser(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for id, sess := range s.sessions {
		if sess.UserID != userID {
			continue
		}
		delete(s.sessions, id)
		deleted++
	}
	return deleted
}
