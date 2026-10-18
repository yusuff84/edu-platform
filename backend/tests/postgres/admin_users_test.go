package postgres_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// TestPostgresListUsersFilters — ListUsers на Postgres: q по email и
// displayName (регистронезависимо), role, status — точные совпадения поверх q.
// Сущности свои и адресуются уникальным токеном, поэтому ожидания — «ровно 1».
func TestPostgresListUsersFilters(t *testing.T) {
	s := testStore(t)

	token := shortID()
	email := runPrefix + "-listusers-" + token + "@example.test"
	u, err := s.CreateUser(email, dummyHash, repo.RoleStudent)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dispToken := "disp" + shortID()
	dispName := "Тестовый Ученик " + dispToken
	if !s.SetUserDisplayName(u.ID, dispName) {
		t.Fatalf("SetUserDisplayName(%s) = false", u.ID)
	}

	// q по email — подстрока без учёта регистра (запрос в верхнем регистре).
	got := s.ListUsers(repo.UserFilter{Q: strings.ToUpper(token)})
	if len(got) != 1 || got[0].ID != u.ID {
		t.Fatalf("ListUsers(q=токен email): %d записей, ожидали 1 (id=%s)", len(got), u.ID)
	}
	if got[0].Status != repo.UserActive || got[0].DisplayName != dispName || got[0].Role != repo.RoleStudent {
		t.Fatalf("ListUsers вернул неполную запись: %+v", got[0])
	}

	// q по displayName — отдельный уникальный токен только в имени.
	got = s.ListUsers(repo.UserFilter{Q: dispToken})
	if len(got) != 1 || got[0].ID != u.ID {
		t.Fatalf("ListUsers(q=токен имени): %d записей, ожидали 1 (id=%s)", len(got), u.ID)
	}

	// role — точное совпадение поверх q.
	if got = s.ListUsers(repo.UserFilter{Q: token, Role: repo.RoleStudent}); len(got) != 1 || got[0].ID != u.ID {
		t.Fatalf("ListUsers(q, role=student): %d записей, ожидали 1", len(got))
	}
	if got = s.ListUsers(repo.UserFilter{Q: token, Role: repo.RoleTeacher}); len(got) != 0 {
		t.Fatalf("ListUsers(q, role=teacher): %d записей, ожидали 0", len(got))
	}

	// status=active — по умолчанию; после блокировки выборки меняются местами.
	if got = s.ListUsers(repo.UserFilter{Q: token, Status: repo.UserActive}); len(got) != 1 {
		t.Fatalf("ListUsers(q, status=active): %d записей, ожидали 1", len(got))
	}
	if !s.SetUserStatus(u.ID, repo.UserBlocked) {
		t.Fatalf("SetUserStatus(blocked) = false")
	}
	got = s.ListUsers(repo.UserFilter{Q: token, Status: repo.UserBlocked})
	if len(got) != 1 || got[0].ID != u.ID || got[0].Status != repo.UserBlocked {
		t.Fatalf("ListUsers(q, status=blocked): %d записей, ожидали 1 со статусом blocked", len(got))
	}
	if got = s.ListUsers(repo.UserFilter{Q: token, Status: repo.UserActive}); len(got) != 0 {
		t.Fatalf("ListUsers(q, status=active) после блокировки: %d записей, ожидали 0", len(got))
	}

	// Разблокировка возвращает active.
	if !s.SetUserStatus(u.ID, repo.UserActive) {
		t.Fatalf("SetUserStatus(active) = false")
	}
	got = s.ListUsers(repo.UserFilter{Q: token, Status: repo.UserActive})
	if len(got) != 1 || got[0].Status != repo.UserActive {
		t.Fatalf("ListUsers(q, status=active) после разблокировки: %d записей, ожидали 1", len(got))
	}

	// Несуществующий пользователь: false, ничего не меняется.
	if s.SetUserStatus(uuid.NewString(), repo.UserBlocked) {
		t.Fatalf("SetUserStatus(несуществующий id) = true, ожидали false")
	}
}

// TestPostgresUserDependenciesAndDelete — связи пользователя (UserDependencies)
// и удаление: свежий пользователь удаляется, повторно — ErrNotFound; удаление
// пользователя со своим курсом отказано (защита UserDependencies + FK
// ON DELETE RESTRICT), связанный пользователь и курс остаются на месте.
func TestPostgresUserDependenciesAndDelete(t *testing.T) {
	s := testStore(t)

	// Свежий пользователь без связей: счётчики нулевые, удаление проходит.
	fresh := createUser(t, s, "deletable", repo.RoleStudent)
	if deps := s.UserDependencies(fresh.ID); deps != (repo.UserDeps{}) {
		t.Fatalf("UserDependencies свежего пользователя: %+v, ожидали нули", deps)
	}
	if err := s.DeleteUser(fresh.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, ok := s.GetUserByID(fresh.ID); ok {
		t.Fatalf("пользователь %s остался в БД после DeleteUser", fresh.ID)
	}
	if err := s.DeleteUser(fresh.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("повторное DeleteUser: err=%v, ожидали ErrNotFound", err)
	}

	// Преподаватель со своим курсом: связь видна, удаление отказано.
	teacher := createUser(t, s, "with-course", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "dep-course", 0)
	if deps := s.UserDependencies(teacher.ID); deps.Courses != 1 || deps.Payments != 0 || deps.Enrollments != 0 {
		t.Fatalf("UserDependencies преподавателя: %+v, ожидали Courses=1", deps)
	}
	if err := s.DeleteUser(teacher.ID); err == nil {
		t.Fatalf("DeleteUser удалил пользователя со своим курсом, ожидали отказ")
	}
	if _, ok := s.GetUserByID(teacher.ID); !ok {
		t.Fatalf("пользователь %s исчез после отказанного удаления", teacher.ID)
	}
	if _, ok := s.GetCourseByID(course.ID); !ok {
		t.Fatalf("курс %s исчез после отказанного удаления", course.ID)
	}

	// Связи ученика: запись на курс и платёж видны в счётчиках.
	student := createUser(t, s, "dep-student", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)
	createPayment(t, s, student.ID, course.ID, 1000)
	if deps := s.UserDependencies(student.ID); deps.Courses != 0 || deps.Payments != 1 || deps.Enrollments != 1 {
		t.Fatalf("UserDependencies ученика: %+v, ожидали Payments=1, Enrollments=1", deps)
	}

	// CountUsersByRole: новый админ увеличивает счётчик ровно на 1.
	before := s.CountUsersByRole(repo.RoleAdmin)
	createUser(t, s, "count-admin", repo.RoleAdmin)
	if got := s.CountUsersByRole(repo.RoleAdmin); got != before+1 {
		t.Fatalf("CountUsersByRole(admin): было %d, стало %d, ожидали +1", before, got)
	}
}
