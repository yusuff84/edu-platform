package postgres_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"edu-platform/backend/internal/migrate"
	"edu-platform/backend/internal/repo"
)

func TestReviewPostgresPaymentOutcomeRollback(t *testing.T) {
	s := testStore(t)
	_, course, _ := seedCourseWithLessons(t, s, 1)
	student := createUser(t, s, "review-billing", repo.RoleStudent)
	promo, err := s.CreatePromoCode(&repo.PromoCode{CourseID: course.ID, Code: "REVIEW", DiscountPct: 20, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePayment(&repo.Payment{StudentID: student.ID, CourseID: course.ID, AmountCents: 8000, PlatformFeeCents: 1200, TeacherAmountCents: 6800, Currency: "RUB", Provider: "mock", Status: repo.PaymentPending, PromoCode: promo.Code, Description: "stable description", ReturnURL: "https://example.com/return"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPaymentCheckout(p.ID, "review_"+p.ID, "https://example.com/pay"); err != nil {
		t.Fatal(err)
	}
	persisted, _ := s.GetPaymentByID(p.ID)
	if persisted.RedirectURL != "https://example.com/pay" || persisted.Description != p.Description || persisted.ReturnURL != p.ReturnURL {
		t.Fatalf("checkout not persisted: %+v", persisted)
	}
	ctx := context.Background()
	name := "review_fail_" + shortID()
	ddl := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.student_id='%s'::uuid THEN RAISE EXCEPTION 'injected enrollment failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER %s BEFORE INSERT OR UPDATE ON enrollments FOR EACH ROW EXECUTE FUNCTION %s()`, name, student.ID, name, name)
	if _, err = s.Pool().Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	defer s.Pool().Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON enrollments; DROP FUNCTION IF EXISTS %s()`, name, name))
	if _, err = s.ApplyPaymentOutcome(p.ID, "succeeded"); err == nil {
		t.Fatal("expected injected failure")
	}
	if current, _ := s.GetPaymentByID(p.ID); current.Status != repo.PaymentPending {
		t.Fatal("payment committed despite enrollment failure")
	}
	if current, _ := s.GetPromoCodeByID(promo.ID); current.UsedCount != 0 {
		t.Fatal("promo committed despite enrollment failure")
	}
	if _, err = s.Pool().Exec(ctx, fmt.Sprintf(`DROP TRIGGER %s ON enrollments`, name)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = s.ApplyPaymentOutcome(p.ID, "succeeded"); err != nil {
			t.Fatal(err)
		}
	}
	if current, _ := s.GetPromoCodeByID(promo.ID); current.UsedCount != 1 {
		t.Fatalf("promo counted twice: %+v", current)
	}
	if err = s.SetEnrollmentStatus(student.ID, course.ID, repo.EnrollmentRevoked); err != nil {
		t.Fatal(err)
	}
	s.ApplyPaymentOutcome(p.ID, "succeeded")
	if e, _ := s.GetEnrollment(student.ID, course.ID); e.Status != repo.EnrollmentRevoked {
		t.Fatal("duplicate undid manual revoke")
	}
	s.SetEnrollmentStatus(student.ID, course.ID, repo.EnrollmentActive)
	s.ApplyPaymentOutcome(p.ID, "refunded")
	second, err := s.CreatePayment(&repo.Payment{StudentID: student.ID, CourseID: course.ID, AmountCents: 10000, PlatformFeeCents: 1500, TeacherAmountCents: 8500, Currency: "RUB", Provider: "mock", Status: repo.PaymentPending})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyPaymentOutcome(second.ID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyPaymentOutcome(p.ID, "refunded"); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.GetEnrollment(student.ID, course.ID); e.Status != repo.EnrollmentActive {
		t.Fatal("old refund revoked newer purchase")
	}
}

func TestReviewPostgresBillingLockAcrossStores(t *testing.T) {
	s := testStore(t)
	unlock, err := s.AcquireBillingLock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	second := repo.NewPostgresStore(s.Pool(), 15)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if release, err := second.AcquireBillingLock(ctx); err == nil {
		release()
		t.Fatal("second store acquired lock while held")
	}
}

func TestReviewMigrationHistoryAndConcurrency(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	name := "review_migration_" + shortID()
	file := name + ".sql"
	path := filepath.Join(dir, file)
	sql := fmt.Sprintf(`CREATE TABLE %s(value int); INSERT INTO %s VALUES(1);`, name, name)
	if err := os.WriteFile(path, []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	defer s.Pool().Exec(context.Background(), `DELETE FROM schema_migrations WHERE name=$1`, file)
	defer s.Pool().Exec(context.Background(), `DROP TABLE IF EXISTS `+name)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- migrate.ApplyMigrations(context.Background(), s.Pool(), dir) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM `+name).Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration replay: count=%d err=%v", count, err)
	}
	if err := os.WriteFile(path, []byte(sql+" -- changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrate.ApplyMigrations(context.Background(), s.Pool(), dir); err == nil {
		t.Fatal("changed applied migration accepted")
	}
}
