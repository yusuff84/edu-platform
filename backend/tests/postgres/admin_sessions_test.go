package postgres_test

// T93 — parity SQL-ветки управления сессиями пользователя: только не
// истёкшие сессии, новые первыми, удаление по user_id с количеством,
// неприкосновенность чужих сессий.

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// TestPostgresAdminSessions — ListSessionsByUser/DeleteSessionsByUser на
// Postgres: фильтр expires_at > now(), порядок created_at DESC, срезы не
// nil, удаление всех сессий (включая истёкшие) и изоляция по user_id.
func TestPostgresAdminSessions(t *testing.T) {
	s := testStore(t)

	user := createUser(t, s, "sessions-user", repo.RoleStudent)
	other := createUser(t, s, "sessions-other", repo.RoleStudent)

	// У пользователя ещё нет сессий: пустой не-nil срез.
	empty := s.ListSessionsByUser(user.ID)
	if empty == nil {
		t.Fatal("ListSessionsByUser вернул nil, ожидали пустой срез")
	}
	if len(empty) != 0 {
		t.Fatalf("ListSessionsByUser до создания: %d сессий, ожидали 0", len(empty))
	}
	if n := s.DeleteSessionsByUser(user.ID); n != 0 {
		t.Fatalf("DeleteSessionsByUser без сессий: %d, ожидали 0", n)
	}

	// Две активные сессии пользователя (с паузой — порядок должен быть
	// однозначным: created_at хранится с микросекундной точностью).
	first := s.CreateSession(user.ID, time.Hour)
	time.Sleep(20 * time.Millisecond)
	second := s.CreateSession(user.ID, 2*time.Hour)
	// Истёкшая сессия и сессия другого пользователя.
	expired := s.CreateSession(user.ID, -time.Minute)
	otherSess := s.CreateSession(other.ID, time.Hour)

	if first.ID == "" || second.ID == "" || expired.ID == "" || otherSess.ID == "" {
		t.Fatalf("CreateSession вернул пустой id: %+v %+v %+v %+v", first, second, expired, otherSess)
	}

	// Только активные целевого пользователя, новые первыми.
	got := s.ListSessionsByUser(user.ID)
	if len(got) != 2 {
		t.Fatalf("ListSessionsByUser: %d сессий, ожидали 2 (истёкшая не в счёт): %+v", len(got), got)
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("порядок сессий: got[0]=%s got[1]=%s, ожидали %s, %s",
			got[0].ID, got[1].ID, second.ID, first.ID)
	}
	for i, sess := range got {
		if sess.UserID != user.ID {
			t.Fatalf("got[%d].UserID=%s, ожидали %s", i, sess.UserID, user.ID)
		}
		if sess.ID == expired.ID {
			t.Fatalf("истёкшая сессия %s попала в список", expired.ID)
		}
		if !sess.ExpiresAt.After(time.Now().UTC()) {
			t.Fatalf("got[%d].ExpiresAt=%v уже в прошлом", i, sess.ExpiresAt)
		}
		if !sess.ExpiresAt.After(sess.CreatedAt) {
			t.Fatalf("got[%d]: expiresAt %v не позже createdAt %v", i, sess.ExpiresAt, sess.CreatedAt)
		}
	}
	// Времена из БД совпадают с созданными (микросекундная точность).
	if !sameMicro(got[0].CreatedAt, second.CreatedAt) || !sameMicro(got[0].ExpiresAt, second.ExpiresAt) {
		t.Fatalf("createdAt/expiresAt второй сессии не сохранены: got=%+v want=%+v", got[0], second)
	}
	if !sameMicro(got[1].CreatedAt, first.CreatedAt) {
		t.Fatalf("createdAt первой сессии не сохранён: got=%v want=%v", got[1].CreatedAt, first.CreatedAt)
	}

	// Чужая сессия в список не попадает.
	if otherList := s.ListSessionsByUser(other.ID); len(otherList) != 1 || otherList[0].ID != otherSess.ID {
		t.Fatalf("сессии другого пользователя: %+v, ожидали только %s", otherList, otherSess.ID)
	}

	// Удаление всех сессий пользователя: обе активные + истёкшая.
	if n := s.DeleteSessionsByUser(user.ID); n != 3 {
		t.Fatalf("DeleteSessionsByUser: %d, ожидали 3 (2 активные + истёкшая)", n)
	}
	if left := s.ListSessionsByUser(user.ID); left == nil || len(left) != 0 {
		t.Fatalf("после удаления остались сессии: %+v", left)
	}
	if _, ok := s.GetSession(first.ID); ok {
		t.Fatalf("сессия %s всё ещё читается после удаления", first.ID)
	}
	// Повторное удаление — 0, не ошибка.
	if n := s.DeleteSessionsByUser(user.ID); n != 0 {
		t.Fatalf("повторное удаление: %d, ожидали 0", n)
	}

	// Чужая сессия не тронута.
	if _, ok := s.GetSession(otherSess.ID); !ok {
		t.Fatalf("сессия другого пользователя %s исчезла", otherSess.ID)
	}

	// Несуществующий пользователь: пустой не-nil список.
	if got := s.ListSessionsByUser(uuid.NewString()); got == nil || len(got) != 0 {
		t.Fatalf("ListSessionsByUser для несуществующего id: %+v", got)
	}
}
