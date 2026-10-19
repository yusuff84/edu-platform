package repo

// In-memory реализация методов раздела «Пользователи» супер-админки (T24).
// Вынесены в отдельный файл, как update.go/postgres_update.go у T13/T19,
// чтобы не раздувать store.go.

import (
	"sort"
	"strings"
)

// normalizedStatus трактует пустой статус как active: так называемые
// «старые объекты» in-memory хранилища создавались до появления поля (T24).
// CreateUser теперь всегда проставляет UserActive явно.
func normalizedStatus(st UserStatus) UserStatus {
	if st == "" {
		return UserActive
	}
	return st
}

// ListUsers — фильтрованный список пользователей для админки (T24).
// Q — подстрока по email/displayName (регистронезависимо), Role/Status —
// точное совпадение, "" — любой. Новые первыми (CreatedAt desc).
func (s *Store) ListUsers(f UserFilter) []*User {
	q := strings.ToLower(strings.TrimSpace(f.Q))
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*User
	for _, u := range s.users {
		if f.Role != "" && u.Role != f.Role {
			continue
		}
		if f.Status != "" && normalizedStatus(u.Status) != f.Status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(u.Email+" "+u.DisplayName), q) {
			continue
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// SetUserStatus активен/заблокирован; false — пользователя нет (T24).
func (s *Store) SetUserStatus(id string, status UserStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return false
	}
	u.Status = status
	return true
}

// DeleteUser удаляет пользователя (T24); ErrNotFound — нет такого.
// Каскад повторяет поведение PostgresStore: сессии, прогресс, отзывы и
// записи на курсы чистятся; курсы и платежи НЕ удаляются — handler обязан
// отказать (409) при их наличии (см. UserDependencies).
func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	for email, u := range s.usersByEmail {
		if u.ID == id {
			delete(s.usersByEmail, email)
		}
	}
	for sid, sess := range s.sessions {
		if sess.UserID == id {
			delete(s.sessions, sid)
		}
	}
	for key := range s.progress {
		if strings.HasPrefix(key, id+"|") {
			delete(s.progress, key)
		}
	}
	for rid, r := range s.reviews {
		if r.StudentID == id {
			delete(s.reviewIdx, enrollKey(r.StudentID, r.CourseID))
			delete(s.reviews, rid)
		}
	}
	for eid, e := range s.enrollments {
		if e.StudentID == id {
			delete(s.enrollIdx, enrollKey(e.StudentID, e.CourseID))
			delete(s.enrollments, eid)
		}
	}
	// T67: сертификаты ученика уходят каскадом (010_certificates.sql).
	s.dropCertificatesByUserLocked(id)
	return nil
}

// UserDependencies — счётчики связей пользователя (T24): курсы, где он
// преподаватель; его платежи; его записи на курсы.
func (s *Store) UserDependencies(id string) UserDeps {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var deps UserDeps
	for _, c := range s.courses {
		if c.TeacherID == id {
			deps.Courses++
		}
	}
	for _, p := range s.payments {
		if p.StudentID == id {
			deps.Payments++
		}
	}
	for _, e := range s.enrollments {
		if e.StudentID == id {
			deps.Enrollments++
		}
	}
	return deps
}

// CountUsersByRole — сколько пользователей с ролью (T24: защита последнего
// администратора в handler'е). Заблокированные админы тоже считаются:
// роль с них снимается отдельным действием.
func (s *Store) CountUsersByRole(role Role) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, u := range s.users {
		if u.Role == role {
			n++
		}
	}
	return n
}
