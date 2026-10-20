package repo

// T73 — избранные курсы: in-memory реализация. Postgres-зеркало —
// postgres_favorites.go; контракт методов — StoreAPI в store_iface.go.
// В избранное попадают любые существующие курсы; фильтр published
// применяется только при чтении списка (контракт GET /me/favorites).

import (
	"sort"
	"time"
)

// AddFavorite идемпотентно добавляет курс в избранное: повторный вызов не
// создаёт дубликат и не сдвигает время добавления. ErrNotFound — нет
// пользователя или курса (в Postgres — нарушение FK).
func (s *Store) AddFavorite(userID, courseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return ErrNotFound
	}
	if _, ok := s.courses[courseID]; !ok {
		return ErrNotFound
	}
	favs := s.favorites[userID]
	if favs == nil {
		favs = make(map[string]time.Time)
		s.favorites[userID] = favs
	}
	if _, exists := favs[courseID]; !exists {
		favs[courseID] = time.Now().UTC()
	}
	return nil
}

// RemoveFavorite идемпотентно удаляет курс из избранного; отсутствие пары —
// успех (в т.ч. если курса уже нет).
func (s *Store) RemoveFavorite(userID, courseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	favs, ok := s.favorites[userID]
	if !ok {
		return nil
	}
	delete(favs, courseID)
	if len(favs) == 0 {
		delete(s.favorites, userID)
	}
	return nil
}

// ListFavoriteCoursesByUser — published-курсы избранного, добавленные позже
// первыми (при равном времени — по id по убыванию, чтобы порядок был
// детерминированным). Никогда не nil.
func (s *Store) ListFavoriteCoursesByUser(userID string) []*Course {
	s.mu.RLock()
	defer s.mu.RUnlock()
	favs := s.favorites[userID]
	out := make([]*Course, 0, len(favs))
	for courseID := range favs {
		c, ok := s.courses[courseID]
		if !ok || c.Status != CoursePublished {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := favs[out[i].ID], favs[out[j].ID]
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// FavoriteCourseIDs возвращает множество id избранных курсов пользователя.
// Нужен публичным видам курса для поля isFavorite; никогда не nil.
func (s *Store) FavoriteCourseIDs(userID string) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	favs := s.favorites[userID]
	out := make(map[string]bool, len(favs))
	for courseID := range favs {
		out[courseID] = true
	}
	return out
}

// dropFavoritesByUserLocked — каскад удаления пользователя
// (зеркало ON DELETE CASCADE 013_favorites.sql). Вызывается с s.mu.
func (s *Store) dropFavoritesByUserLocked(userID string) {
	delete(s.favorites, userID)
}

// dropFavoritesByCourseLocked — каскад удаления курса
// (зеркало ON DELETE CASCADE 013_favorites.sql). Вызывается с s.mu.
func (s *Store) dropFavoritesByCourseLocked(courseID string) {
	for userID, favs := range s.favorites {
		delete(favs, courseID)
		if len(favs) == 0 {
			delete(s.favorites, userID)
		}
	}
}
