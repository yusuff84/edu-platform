package repo

// Юнит-тесты in-memory стора по точкам, закрепляемым T20:
//   - SetEnrollmentStatus: смена статуса и ErrNotFound (T16, п.5/п.6);
//   - EnsureEnrollment: refunded/revoked → active при успешной оплате;
//   - ListPaymentsJournal: клампинг limit, items != null, totals без пагинации.
// Postgres-вариант того же интерфейса без живой БД не тестируется —
// см. отчёт T20 («Что тесты НЕ покрывают»).

import (
	"errors"
	"testing"
)

// TestSetEnrollmentStatus — смена статуса существующей записи и ErrNotFound
// для отсутствующей (контракт для handler'а revoke/unrevoke и webhook'а).
func TestSetEnrollmentStatus(t *testing.T) {
	s := NewStore()
	u, err := s.CreateUser("st@example.com", "хеш", RoleStudent)
	if err != nil {
		t.Fatalf("создание пользователя: %v", err)
	}
	c := s.CreateCourse(&Course{TeacherID: "t1", Slug: "status-course", Title: "Курс", Description: "x"})

	// Записи нет — ErrNotFound (а не паника и не nil).
	if err := s.SetEnrollmentStatus(u.ID, c.ID, EnrollmentRevoked); !errors.Is(err, ErrNotFound) {
		t.Fatalf("нет записи: err=%v, ждали ErrNotFound", err)
	}

	// Создаём запись, меняем статус во все направления.
	e, created := s.EnsureEnrollment(u.ID, c.ID)
	if !created || e.Status != EnrollmentActive {
		t.Fatalf("EnsureEnrollment: created=%v status=%v", created, e.Status)
	}
	for _, st := range []EnrollmentStatus{EnrollmentRevoked, EnrollmentActive, EnrollmentRefunded} {
		if err := s.SetEnrollmentStatus(u.ID, c.ID, st); err != nil {
			t.Fatalf("SetEnrollmentStatus(%s): %v", st, err)
		}
		if got, ok := s.GetEnrollment(u.ID, c.ID); !ok || got.Status != st {
			t.Fatalf("после SetEnrollmentStatus(%s): %+v", st, got)
		}
	}
}

// TestEnsureEnrollmentReactivates — T16 (п.5): EnsureEnrollment вызывается
// только на путях УСПЕШНОГО платежа, поэтому существующая запись в
// refunded/revoked обязана стать active — иначе оплата прошла, а доступ
// не открылся.
func TestEnsureEnrollmentReactivates(t *testing.T) {
	s := NewStore()
	u, _ := s.CreateUser("re@example.com", "хеш", RoleStudent)
	c := s.CreateCourse(&Course{TeacherID: "t1", Slug: "reactivate-course", Title: "Курс", Description: "x"})

	for _, from := range []EnrollmentStatus{EnrollmentRefunded, EnrollmentRevoked} {
		s.EnsureEnrollment(u.ID, c.ID)
		if err := s.SetEnrollmentStatus(u.ID, c.ID, from); err != nil {
			t.Fatalf("подготовка (%s): %v", from, err)
		}
		e, created := s.EnsureEnrollment(u.ID, c.ID)
		if created {
			t.Fatalf("повторный EnsureEnrollment не должен создавать новую запись (%s)", from)
		}
		if e.Status != EnrollmentActive {
			t.Fatalf("%s → active не сработало: %+v", from, e)
		}
	}
}

// TestListPaymentsJournalClampAndTotals — клампинг limit на уровне стора
// (<=0 → 20, >100 → 100), items — всегда непустой слайс (не nil) и totals,
// посчитанные по всем записям независимо от страницы.
func TestListPaymentsJournalClampAndTotals(t *testing.T) {
	s := NewStore()
	u, _ := s.CreateUser("j@example.com", "хеш", RoleStudent)
	c := s.CreateCourse(&Course{TeacherID: "t1", Slug: "journal-course", Title: "Курс", Description: "x"})

	// Пустой журнал: items != nil (JSON — []), totals нулевые.
	rows, total, totals, err := s.ListPaymentsJournal(PaymentFilter{})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("пустой журнал: rows=%v total=%d err=%v", rows, total, err)
	}
	if rows == nil {
		t.Fatal("items должен быть пустым слайсом, а не nil (иначе в JSON — null)")
	}
	if totals.AmountCents != 0 || totals.PlatformFeeCents != 0 || totals.TeacherAmountCents != 0 {
		t.Fatalf("totals пустого журнала: %+v", totals)
	}

	// Пять платежей по 1000 (сплит 15%/85% при комиссии 15).
	// CreatedAt генерируется стором (time.Now), порядок в тесте не важен —
	// проверяются только клампинг и totals.
	for i := 0; i < 5; i++ {
		if _, err := s.CreatePayment(&Payment{
			StudentID: u.ID, CourseID: c.ID, AmountCents: 1000,
			PlatformFeeCents: 150, TeacherAmountCents: 850,
			Currency: "RUB", Provider: "mock",
			ProviderPaymentID: "j-" + string(rune('a'+i)),
			Status:            PaymentSucceeded,
		}); err != nil {
			t.Fatalf("создание платежа %d: %v", i, err)
		}
	}

	// Клампинг limit.
	for _, cse := range []struct {
		limit, want int
	}{{0, 20}, {-3, 20}, {1, 1}, {3, 3}, {100, 100}, {500, 100}} {
		rows, total, _, err := s.ListPaymentsJournal(PaymentFilter{Limit: cse.limit})
		if err != nil {
			t.Fatalf("limit=%d: %v", cse.limit, err)
		}
		wantRows := cse.want
		if wantRows > 5 {
			wantRows = 5 // записей всего 5
		}
		if len(rows) != wantRows {
			t.Fatalf("limit=%d: %d строк, ждали %d", cse.limit, len(rows), wantRows)
		}
		if total != 5 {
			t.Fatalf("limit=%d: total=%d, пагинация не должна менять total", cse.limit, total)
		}
	}

	// Totals не зависят от страницы: сумма всех 5 платежей.
	for _, page := range []int{1, 2, 5} {
		_, _, totals, err := s.ListPaymentsJournal(PaymentFilter{Page: page, Limit: 1})
		if err != nil {
			t.Fatalf("page=%d: %v", page, err)
		}
		if totals.AmountCents != 5000 || totals.PlatformFeeCents != 750 || totals.TeacherAmountCents != 4250 {
			t.Fatalf("page=%d: totals=%+v, ждали сумму всех записей", page, totals)
		}
	}

	// Фильтр по статусу: pending-платежей нет.
	rows, total, _, err = s.ListPaymentsJournal(PaymentFilter{Status: PaymentPending})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("фильтр pending: rows=%d total=%d err=%v", len(rows), total, err)
	}
}
