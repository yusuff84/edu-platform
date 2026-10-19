package postgres_test

// Интеграционные тесты SQL-ветки промокодов (T63): CRUD и parity с
// in-memory Store, поля платежа и строка журнала админа. Env-gated как
// остальные тесты пакета: без TEST_DATABASE_URL — t.Skip.

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// TestPostgresPromoCodesCRUD — создание (нормализация кода, значения по
// умолчанию, срок), дубликат, список новыми первыми, поиск по id и по
// (courseId, code), идемпотентная деактивация, атомарный инкремент
// usedCount и каскад при удалении курса.
func TestPostgresPromoCodesCRUD(t *testing.T) {
	s := testStore(t)

	teacher, course, _ := seedCourseWithLessons(t, s, 1)

	expires := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Microsecond)
	p1, err := s.CreatePromoCode(&repo.PromoCode{
		CourseID: course.ID, Code: " sale20 ", DiscountPct: 20,
		MaxUses: 100, ExpiresAt: &expires,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode: %v", err)
	}
	if p1.ID == "" || p1.Code != "SALE20" || p1.CourseID != course.ID ||
		p1.DiscountPct != 20 || p1.MaxUses != 100 || p1.UsedCount != 0 || !p1.Active {
		t.Fatalf("созданный промокод: %+v", p1)
	}
	if p1.ExpiresAt == nil || !sameMicro(*p1.ExpiresAt, expires) {
		t.Fatalf("expiresAt: got=%v want=%v", p1.ExpiresAt, expires)
	}
	if p1.CreatedAt.IsZero() {
		t.Fatal("createdAt не заполнен")
	}

	// Дубликат (course_id, code) без учёта регистра — ErrAlreadyExists.
	if _, err := s.CreatePromoCode(&repo.PromoCode{
		CourseID: course.ID, Code: "sale20", DiscountPct: 10,
	}); !errors.Is(err, repo.ErrAlreadyExists) {
		t.Fatalf("дубликат: err=%v, ждали ErrAlreadyExists", err)
	}

	// Второй код без срока и лимита: maxUses=0, expiresAt=NULL.
	time.Sleep(2 * time.Millisecond)
	p2, err := s.CreatePromoCode(&repo.PromoCode{
		CourseID: course.ID, Code: "WELCOME", DiscountPct: 5,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode (WELCOME): %v", err)
	}
	if p2.MaxUses != 0 || p2.ExpiresAt != nil {
		t.Fatalf("значения по умолчанию: %+v", p2)
	}

	// Список: только коды курса, новые первыми.
	items := s.ListPromoCodesByCourse(course.ID)
	if len(items) != 2 || items[0].ID != p2.ID || items[1].ID != p1.ID {
		t.Fatalf("список курса: %+v", items)
	}
	if empty := s.ListPromoCodesByCourse(uuid.NewString()); empty == nil || len(empty) != 0 {
		t.Fatalf("пустой список должен быть [] (не nil): %#v", empty)
	}

	// Поиск по (courseId, code) — регистр не важен.
	found, ok := s.FindPromoCode(course.ID, "sale20")
	if !ok || found.ID != p1.ID || found.ExpiresAt == nil {
		t.Fatalf("FindPromoCode: ok=%v got=%+v", ok, found)
	}
	if _, ok := s.FindPromoCode(uuid.NewString(), "SALE20"); ok {
		t.Fatal("FindPromoCode нашёл код чужого курса")
	}
	if _, ok := s.FindPromoCode(course.ID, "NOPE"); ok {
		t.Fatal("FindPromoCode нашёл несуществующий код")
	}
	// Получение по id; мусорный id — «не найдено», без паники.
	got, ok := s.GetPromoCodeByID(p1.ID)
	if !ok || got.Code != "SALE20" {
		t.Fatalf("GetPromoCodeByID: ok=%v got=%+v", ok, got)
	}
	if _, ok := s.GetPromoCodeByID(uuid.NewString()); ok {
		t.Fatal("GetPromoCodeByID нашёл несуществующий id")
	}
	if _, ok := s.GetPromoCodeByID("not-a-uuid"); ok {
		t.Fatal("GetPromoCodeByID не должен падать/находить мусорный id")
	}

	// Деактивация идемпотентна, запись остаётся.
	if err := s.DeactivatePromoCode(p1.ID); err != nil {
		t.Fatalf("DeactivatePromoCode: %v", err)
	}
	if err := s.DeactivatePromoCode(p1.ID); err != nil {
		t.Fatalf("повторная DeactivatePromoCode: %v", err)
	}
	if got, _ := s.GetPromoCodeByID(p1.ID); got == nil || got.Active {
		t.Fatalf("после деактивации: %+v", got)
	}
	if err := s.DeactivatePromoCode(uuid.NewString()); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("деактивация несуществующего: err=%v, ждали ErrNotFound", err)
	}
	if err := s.DeactivatePromoCode("not-a-uuid"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("деактивация мусорного id: err=%v, ждали ErrNotFound", err)
	}

	// Атомарный инкремент usedCount.
	if !s.IncrementPromoCodeUsage(p1.ID) || !s.IncrementPromoCodeUsage(p1.ID) {
		t.Fatal("IncrementPromoCodeUsage вернул false для существующего кода")
	}
	if got, _ := s.GetPromoCodeByID(p1.ID); got == nil || got.UsedCount != 2 {
		t.Fatalf("usedCount: %+v, ждали 2", got)
	}
	if s.IncrementPromoCodeUsage(uuid.NewString()) {
		t.Fatal("IncrementPromoCodeUsage вернул true для несуществующего кода")
	}

	// Удаление курса каскадом удаляет его промокоды (008_promo_codes.sql).
	c2 := createCourse(t, s, teacher.ID, "promo-cascade", 1000)
	pc, err := s.CreatePromoCode(&repo.PromoCode{CourseID: c2.ID, Code: "CASCADE", DiscountPct: 10})
	if err != nil {
		t.Fatalf("CreatePromoCode (CASCADE): %v", err)
	}
	if err := s.DeleteCourse(c2.ID); err != nil {
		t.Fatalf("DeleteCourse: %v", err)
	}
	if _, ok := s.GetPromoCodeByID(pc.ID); ok {
		t.Fatal("промокод удалённого курса остался в БД (нет каскада)")
	}
}

// TestPostgresPaymentPromoFields — аддитивные promo_code/discount_cents
// сохраняются, читаются и попадают в строку журнала админа; у обычного
// платежа promoCode пустой, discountCents = 0.
func TestPostgresPaymentPromoFields(t *testing.T) {
	s := testStore(t)

	_, course, _ := seedCourseWithLessons(t, s, 1)
	student := createUser(t, s, "promo-pay", repo.RoleStudent)

	p, err := s.CreatePayment(&repo.Payment{
		StudentID: student.ID, CourseID: course.ID,
		AmountCents: 8000, PlatformFeeCents: 1200, TeacherAmountCents: 6800,
		Currency: "RUB", Provider: "mock",
		ProviderPaymentID: runPrefix + "-promo-pay-" + shortID(),
		Status:            repo.PaymentPending,
		PromoCode:         "SALE20", DiscountCents: 2000,
	})
	if err != nil {
		t.Fatalf("CreatePayment с промокодом: %v", err)
	}
	got, ok := s.GetPaymentByID(p.ID)
	if !ok || got.PromoCode != "SALE20" || got.DiscountCents != 2000 {
		t.Fatalf("поля промокода платежа: ok=%v got=%+v", ok, got)
	}
	if !s.SetPaymentStatus(p.ID, repo.PaymentSucceeded) {
		t.Fatal("SetPaymentStatus(succeeded) = false")
	}

	// Обычный платёж без кода: пустая строка и 0 (аддитивность).
	plain := createPayment(t, s, student.ID, course.ID, 1000)
	if plain.PromoCode != "" || plain.DiscountCents != 0 {
		t.Fatalf("обычный платёж: promoCode=%q discount=%d", plain.PromoCode, plain.DiscountCents)
	}

	items, total, totals, err := s.ListPaymentsJournal(repo.PaymentFilter{CourseID: course.ID})
	if err != nil {
		t.Fatalf("ListPaymentsJournal: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("журнал: total=%d len=%d, ждали 2", total, len(items))
	}
	var promoRow *repo.PaymentJournalRow
	for i := range items {
		if items[i].ID == p.ID {
			promoRow = &items[i]
		}
	}
	if promoRow == nil {
		t.Fatal("платёж с промокодом не найден в журнале")
	}
	if promoRow.PromoCode != "SALE20" || promoRow.DiscountCents != 2000 || promoRow.AmountCents != 8000 {
		t.Fatalf("строка журнала с промокодом: %+v", promoRow.Payment)
	}
	if totals.AmountCents != 9000 || totals.PlatformFeeCents != 1350 || totals.TeacherAmountCents != 7650 {
		t.Fatalf("totals журнала: %+v", totals)
	}
}
