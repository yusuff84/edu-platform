package postgres_test

// T47 на Postgres: список/отзыв приглашений, поиск платежа по внутреннему id
// и processed_at у терминальных статусов. Env-gated через TEST_DATABASE_URL,
// как остальные тесты пакета.

import (
	"errors"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

func TestPostgresAdminT47Parity(t *testing.T) {
	s := testStore(t)
	admin := createUser(t, s, "t47-admin", repo.RoleAdmin)
	expires := time.Now().UTC().Add(time.Hour)

	first := s.CreateInvite(
		"t47-first-"+shortID()+"@example.com",
		"hash-first-"+shortID(),
		"pref-first",
		admin.ID,
		expires,
	)
	second := s.CreateInvite(
		"t47-second-"+shortID()+"@example.com",
		"hash-second-"+shortID(),
		"pref-second",
		admin.ID,
		expires,
	)
	if first.ID == "" || second.ID == "" {
		t.Fatalf("CreateInvite не вернул id: first=%+v second=%+v", first, second)
	}

	byHash, ok := s.GetInviteByTokenHash(first.TokenHash)
	if !ok || byHash.ID != first.ID || byHash.Status != repo.InviteSent {
		t.Fatalf("GetInviteByTokenHash: ok=%v inv=%+v", ok, byHash)
	}

	items, total := s.ListInvites(1, 100)
	if total < 2 {
		t.Fatalf("ListInvites total=%d, ожидали минимум 2", total)
	}
	found := map[string]bool{}
	for _, inv := range items {
		if inv.ID == first.ID || inv.ID == second.ID {
			found[inv.ID] = true
		}
	}
	if !found[first.ID] || !found[second.ID] {
		t.Fatalf("ListInvites не вернул созданные приглашения: %v", found)
	}

	revoked, err := s.RevokeInvite(first.ID)
	if err != nil || revoked.Status != repo.InviteRevoked {
		t.Fatalf("RevokeInvite: err=%v inv=%+v", err, revoked)
	}
	if _, err := s.RevokeInvite(first.ID); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("повторный RevokeInvite: err=%v, ожидали ErrConflict", err)
	}
	if _, err := s.RevokeInvite("not-a-uuid"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("RevokeInvite(мусор): err=%v, ожидали ErrNotFound", err)
	}

	teacher := createUser(t, s, "t47-teacher", repo.RoleTeacher)
	student := createUser(t, s, "t47-student", repo.RoleStudent)
	course := createCourse(t, s, teacher.ID, "t47-payment-course", 10000)
	p, err := s.CreatePayment(&repo.Payment{
		StudentID:          student.ID,
		CourseID:           course.ID,
		AmountCents:        10000,
		PlatformFeeCents:   1500,
		TeacherAmountCents: 8500,
		Currency:           "RUB",
		Provider:           "mock",
		ProviderPaymentID:  "mock_t47_" + shortID(),
		Status:             repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	got, ok := s.GetPaymentByID(p.ID)
	if !ok || got.ID != p.ID || got.Status != repo.PaymentPending {
		t.Fatalf("GetPaymentByID: ok=%v p=%+v", ok, got)
	}
	if !s.SetPaymentStatus(p.ID, repo.PaymentSucceeded) {
		t.Fatal("SetPaymentStatus(succeeded): false")
	}
	got, _ = s.GetPaymentByID(p.ID)
	if got.Status != repo.PaymentSucceeded || got.ProcessedAt == nil {
		t.Fatalf("после succeeded: %+v", got)
	}
	if !s.SetPaymentStatus(p.ID, repo.PaymentRefunded) {
		t.Fatal("SetPaymentStatus(refunded): false")
	}
	got, _ = s.GetPaymentByID(p.ID)
	if got.Status != repo.PaymentRefunded || got.ProcessedAt == nil {
		t.Fatalf("после refunded: %+v", got)
	}
	if s.SetPaymentStatus("not-a-uuid", repo.PaymentRefunded) {
		t.Fatal("SetPaymentStatus(мусор): true, ожидали false")
	}
}
