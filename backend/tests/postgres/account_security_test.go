package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/repo"
)

func TestAccountSecurityStoreParity(t *testing.T) {
	t.Run("memory", func(t *testing.T) { testAccountSecurityStore(t, repo.NewStore()) })
	t.Run("postgres", func(t *testing.T) { testAccountSecurityStore(t, testStore(t)) })
}

func testAccountSecurityStore(t *testing.T, s repo.StoreAPI) {
	user, err := s.CreateUser(uniqueEmail("account"), "old-hash", repo.RoleStudent)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.CreateUser(uniqueEmail("foreign-account"), "foreign-hash", repo.RoleStudent)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateAccountSession(user.ID, "old-hash", time.Hour, "First browser", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateAccountSession(user.ID, "old-hash", time.Hour, "Second browser", "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicID == "" || first.PublicID == first.ID || second.PublicID == first.PublicID {
		t.Fatal("management IDs must be distinct from authentication credentials")
	}
	if revoked, err := s.RevokeAccountSession(foreign.ID, second.PublicID); err != nil || revoked {
		t.Fatal("foreign user revoked session")
	}
	if err := s.RevokeOtherAccountSessions(user.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetSession(second.ID); ok {
		t.Fatal("other session survived")
	}
	if _, ok := s.GetSession(first.ID); !ok {
		t.Fatal("current session was revoked")
	}
	if _, err = s.CreateAccountSession(user.ID, "wrong-hash", time.Hour, "", ""); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("stale credentials created a session: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, oldHash, _, _ := auth.GenerateToken()
	_, verifyHash, _, _ := auth.GenerateToken()
	if err = s.IssueAccountToken(user.ID, "old-hash", repo.EmailVerification, oldHash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.IssueAccountToken(user.ID, "old-hash", repo.EmailVerification, verifyHash, now.Add(10*time.Second), now.Add(time.Hour)); !errors.Is(err, repo.ErrTokenRateLimited) {
		t.Fatalf("resend cooldown bypassed: %v", err)
	}
	if err = s.IssueAccountToken(user.ID, "old-hash", repo.EmailVerification, verifyHash, now.Add(61*time.Second), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.ConsumeAccountToken(oldHash, repo.EmailVerification, "", now.Add(62*time.Second)); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatal("superseded link works")
	}
	if err = s.ConsumeAccountToken(verifyHash, repo.PasswordReset, "new-hash", now.Add(62*time.Second)); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatal("wrong token purpose accepted")
	}
	if err = s.ConsumeAccountToken(verifyHash, repo.EmailVerification, "", now.Add(62*time.Second)); err != nil {
		t.Fatal(err)
	}
	if refreshed, _ := s.GetUserByID(user.ID); refreshed.EmailVerifiedAt == nil {
		t.Fatal("email not verified")
	}
	_, resetHash, _, _ := auth.GenerateToken()
	if err = s.IssueAccountToken(user.ID, "old-hash", repo.PasswordReset, resetHash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = s.ConsumeAccountToken(resetHash, repo.PasswordReset, "new-hash", now.Add(time.Minute))
		}(i)
	}
	wg.Wait()
	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, repo.ErrInvalidAccountToken) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("reset token consumed %d times", winners)
	}
	if _, ok := s.GetSession(first.ID); ok {
		t.Fatal("reset left old session active")
	}
	if _, err = s.CreateAccountSession(user.ID, "old-hash", time.Hour, "", ""); !errors.Is(err, repo.ErrConflict) {
		t.Fatal("old-password login recreated revoked session")
	}
	if _, err = s.CreateAccountSession(user.ID, "new-hash", time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestAccountResetAndLoginRace(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 5; i++ {
		user, err := s.CreateUser(uniqueEmail("login-race"), "old-hash", repo.RoleStudent)
		if err != nil {
			t.Fatal(err)
		}
		_, hash, _, _ := auth.GenerateToken()
		now := time.Now().UTC()
		if err = s.IssueAccountToken(user.ID, "old-hash", repo.PasswordReset, hash, now, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var resetErr, loginErr error
		go func() {
			defer wg.Done()
			<-start
			_, loginErr = s.CreateAccountSession(user.ID, "old-hash", time.Hour, "", "")
		}()
		go func() {
			defer wg.Done()
			<-start
			resetErr = s.ConsumeAccountToken(hash, repo.PasswordReset, "new-hash", time.Now().UTC())
		}()
		close(start)
		wg.Wait()
		if resetErr != nil {
			t.Fatal(resetErr)
		}
		if loginErr != nil && !errors.Is(loginErr, repo.ErrConflict) {
			t.Fatal(loginErr)
		}
		if sessions, err := s.ListAccountSessions(user.ID); err != nil || len(sessions) != 0 {
			t.Fatalf("racing old-password login survived reset: %d %v", len(sessions), err)
		}
	}
}

func TestAccountLegacySessionsCanBeManaged(t *testing.T) {
	s := testStore(t)
	user := createUser(t, s, "legacy-session", repo.RoleStudent)
	legacy := s.CreateSession(user.ID, time.Hour)
	if legacy == nil {
		t.Fatal("legacy session creation failed")
	}
	current, ok := s.GetSession(legacy.ID)
	if !ok || current.PublicID == "" || current.PublicID == legacy.ID {
		t.Fatal("legacy session has no separate management ID")
	}
	if removed, err := s.RevokeAccountSession(user.ID, current.PublicID); err != nil || !removed {
		t.Fatal("legacy session cannot be revoked")
	}
	if _, ok = s.GetSession(legacy.ID); ok {
		t.Fatal("legacy authentication credential still valid")
	}
}

func TestAccountResetTransactionRollback(t *testing.T) {
	s := testStore(t)
	user := createUser(t, s, "reset-rollback", repo.RoleStudent)
	if _, err := s.CreateAccountSession(user.ID, user.PasswordHash, time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
	_, hash, _, _ := auth.GenerateToken()
	now := time.Now().UTC()
	if err := s.IssueAccountToken(user.ID, user.PasswordHash, repo.PasswordReset, hash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	name := "account_fail_" + shortID()
	ctx := context.Background()
	ddl := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.user_id='%s'::uuid THEN RAISE EXCEPTION 'injected session delete failure'; END IF; RETURN OLD; END $$;
CREATE TRIGGER %s BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION %s()`, name, user.ID, name, name)
	if _, err := s.Pool().Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	defer s.Pool().Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON sessions; DROP FUNCTION IF EXISTS %s()`, name, name))
	if err := s.ConsumeAccountToken(hash, repo.PasswordReset, "new-hash", now); err == nil {
		t.Fatal("expected injected failure")
	}
	if refreshed, _ := s.GetUserByID(user.ID); refreshed.PasswordHash != user.PasswordHash {
		t.Fatal("password committed despite session deletion failure")
	}
	if _, err := s.Pool().Exec(ctx, fmt.Sprintf(`DROP TRIGGER %s ON sessions`, name)); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeAccountToken(hash, repo.PasswordReset, "new-hash", now); err != nil {
		t.Fatalf("rollback consumed the reset token: %v", err)
	}
}

func TestAccountExpiredTokenAndBlockedUserPostgres(t *testing.T) {
	s := testStore(t)
	user := createUser(t, s, "token-expiry", repo.RoleStudent)
	_, hash, _, _ := auth.GenerateToken()
	old := time.Now().Add(-2 * time.Hour)
	if err := s.IssueAccountToken(user.ID, user.PasswordHash, repo.EmailVerification, hash, old, old.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeAccountToken(hash, repo.EmailVerification, "", time.Now()); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatal("expired verification accepted")
	}
	_, hash, _, _ = auth.GenerateToken()
	now := time.Now()
	if err := s.IssueAccountToken(user.ID, user.PasswordHash, repo.EmailVerification, hash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.SetUserStatus(user.ID, repo.UserBlocked)
	if err := s.ConsumeAccountToken(hash, repo.EmailVerification, "", now); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatal("blocked user verified email")
	}
}
