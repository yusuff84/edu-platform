package postgres_test

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestPostgresPaymentsJournalFiltersTotalsAndPaging — ListPaymentsJournal:
// фильтры status/courseId складываются по «И», totals считаются по
// отфильтрованному набору (включая не-succeeded), пагинация 1-based, новые
// первыми, строка несёт название курса и данные студента.
func TestPostgresPaymentsJournalFiltersTotalsAndPaging(t *testing.T) {
	s := testStore(t)

	_, c1, _ := seedCourseWithLessons(t, s, 1)
	_, c2, _ := seedCourseWithLessons(t, s, 0)

	student := createUser(t, s, "journal-student", repo.RoleStudent)
	dispName := "Журнальный Ученик " + runPrefix
	if !s.SetUserDisplayName(student.ID, dispName) {
		t.Fatalf("SetUserDisplayName = false")
	}
	enroll(t, s, student.ID, c1.ID)

	// Три платежа за c1 (создаются pending, статус ставится как на реальном
	// пути оплаты): p1 — succeeded 10000, p2 — failed 5000, p3 — succeeded 2000.
	p1 := createPayment(t, s, student.ID, c1.ID, 10000)
	p2 := createPayment(t, s, student.ID, c1.ID, 5000)
	p3 := createPayment(t, s, student.ID, c1.ID, 2000)
	if !s.SetPaymentStatus(p1.ID, repo.PaymentSucceeded) ||
		!s.SetPaymentStatus(p2.ID, repo.PaymentFailed) ||
		!s.SetPaymentStatus(p3.ID, repo.PaymentSucceeded) {
		t.Fatalf("SetPaymentStatus вернул false")
	}
	// pending-платёж за другой курс: не должен попадать в выборки по c1.
	pOther := createPayment(t, s, student.ID, c2.ID, 100)

	// Все платежи курса c1, новые первыми.
	items, total, totals, err := s.ListPaymentsJournal(repo.PaymentFilter{CourseID: c1.ID})
	if err != nil {
		t.Fatalf("ListPaymentsJournal: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("журнал c1 без фильтра статуса: total=%d len=%d, ожидали 3/3", total, len(items))
	}
	wantOrder := []string{p3.ID, p2.ID, p1.ID}
	for i, id := range wantOrder {
		if items[i].ID != id {
			t.Fatalf("порядок журнала: items[%d]=%s, ожидали %s (новые первыми)", i, items[i].ID, id)
		}
	}
	// Строка: курс, ученик, суммы и сплит.
	row := items[0]
	if row.CourseTitle != c1.Title || row.StudentEmail != student.Email || row.StudentDisplayName != dispName {
		t.Fatalf("строка журнала: title=%q email=%q name=%q", row.CourseTitle, row.StudentEmail, row.StudentDisplayName)
	}
	if row.AmountCents != 2000 || row.PlatformFeeCents != 300 || row.TeacherAmountCents != 1700 {
		t.Fatalf("суммы строки журнала: %+v", row.Payment)
	}
	if row.Status != repo.PaymentSucceeded || row.ProcessedAt == nil {
		t.Fatalf("строка succeeded без processed_at: %+v", row.Payment)
	}
	wantTotals := repo.PaymentTotals{AmountCents: 17000, PlatformFeeCents: 2550, TeacherAmountCents: 14450}
	if totals != wantTotals {
		t.Fatalf("totals без фильтра статуса: %+v, ожидали %+v", totals, wantTotals)
	}

	// status=succeeded: только успешные; totals — по отфильтрованным.
	items, total, totals, err = s.ListPaymentsJournal(repo.PaymentFilter{CourseID: c1.ID, Status: repo.PaymentSucceeded})
	if err != nil {
		t.Fatalf("ListPaymentsJournal(succeeded): %v", err)
	}
	if total != 2 || len(items) != 2 || items[0].ID != p3.ID || items[1].ID != p1.ID {
		t.Fatalf("журнал succeeded: total=%d ids=%v, ожидали 2 (p3, p1)", total, idsOf(items))
	}
	wantTotals = repo.PaymentTotals{AmountCents: 12000, PlatformFeeCents: 1800, TeacherAmountCents: 10200}
	if totals != wantTotals {
		t.Fatalf("totals succeeded: %+v, ожидали %+v", totals, wantTotals)
	}

	// status=failed: один платёж.
	items, total, totals, err = s.ListPaymentsJournal(repo.PaymentFilter{CourseID: c1.ID, Status: repo.PaymentFailed})
	if err != nil {
		t.Fatalf("ListPaymentsJournal(failed): %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != p2.ID {
		t.Fatalf("журнал failed: total=%d ids=%v, ожидали 1 (p2)", total, idsOf(items))
	}
	wantTotals = repo.PaymentTotals{AmountCents: 5000, PlatformFeeCents: 750, TeacherAmountCents: 4250}
	if totals != wantTotals {
		t.Fatalf("totals failed: %+v, ожидали %+v", totals, wantTotals)
	}

	// Пагинация: limit=2 — страница 1 из двух строк, страница 2 — одна.
	items, total, _, err = s.ListPaymentsJournal(repo.PaymentFilter{CourseID: c1.ID, Page: 1, Limit: 2})
	if err != nil || total != 3 || len(items) != 2 || items[0].ID != p3.ID || items[1].ID != p2.ID {
		t.Fatalf("страница 1 (limit=2): total=%d ids=%v err=%v", total, idsOf(items), err)
	}
	items, total, _, err = s.ListPaymentsJournal(repo.PaymentFilter{CourseID: c1.ID, Page: 2, Limit: 2})
	if err != nil || total != 3 || len(items) != 1 || items[0].ID != p1.ID {
		t.Fatalf("страница 2 (limit=2): total=%d ids=%v err=%v", total, idsOf(items), err)
	}

	// Второй курс: только свой pending, ещё не обработанный.
	items, total, totals, err = s.ListPaymentsJournal(repo.PaymentFilter{CourseID: c2.ID})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != pOther.ID {
		t.Fatalf("журнал c2: total=%d len=%d err=%v", total, len(items), err)
	}
	if items[0].ProcessedAt != nil || items[0].Status != repo.PaymentPending {
		t.Fatalf("pending-платёж c2: %+v", items[0].Payment)
	}
	wantTotals = repo.PaymentTotals{AmountCents: 100, PlatformFeeCents: 15, TeacherAmountCents: 85}
	if totals != wantTotals {
		t.Fatalf("totals c2: %+v, ожидали %+v", totals, wantTotals)
	}
}

// idsOf — id строк журнала для сообщений об ошибках.
func idsOf(items []repo.PaymentJournalRow) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}
