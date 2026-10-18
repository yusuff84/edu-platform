package postgres_test

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestPostgresAdminOverviewUsersBlocked — главная проверка фикса T34:
// PostgresStore.AdminOverview считает users.blocked. Сводка глобальная
// (общая база), поэтому все ожидания — дельты от снимка «до».
func TestPostgresAdminOverviewUsersBlocked(t *testing.T) {
	s := testStore(t)

	before := s.AdminOverview()

	// Создание ученика: users.total и users.students растут, blocked — нет.
	student := createUser(t, s, "stats-student", repo.RoleStudent)
	created := s.AdminOverview()
	if created.UsersTotal != before.UsersTotal+1 {
		t.Fatalf("UsersTotal после создания: было %d, стало %d, ожидали +1", before.UsersTotal, created.UsersTotal)
	}
	if created.UsersStudents != before.UsersStudents+1 {
		t.Fatalf("UsersStudents после создания: было %d, стало %d, ожидали +1", before.UsersStudents, created.UsersStudents)
	}
	if created.UsersBlocked != before.UsersBlocked {
		t.Fatalf("UsersBlocked до блокировки: было %d, стало %d", before.UsersBlocked, created.UsersBlocked)
	}

	// Блокировка: users.blocked увеличивается ровно на 1. До фикса T34 здесь
	// всегда было 0 — SQL-запрос не выбирал status='blocked'.
	if !s.SetUserStatus(student.ID, repo.UserBlocked) {
		t.Fatalf("SetUserStatus(blocked) = false")
	}
	blocked := s.AdminOverview()
	if blocked.UsersBlocked != before.UsersBlocked+1 {
		t.Fatalf("UsersBlocked после блокировки: было %d, стало %d, ожидали +1 — фикс T34 не работает",
			before.UsersBlocked, blocked.UsersBlocked)
	}

	// Разблокировка: счётчик возвращается к исходному значению.
	if !s.SetUserStatus(student.ID, repo.UserActive) {
		t.Fatalf("SetUserStatus(active) = false")
	}
	if got := s.AdminOverview().UsersBlocked; got != before.UsersBlocked {
		t.Fatalf("UsersBlocked после разблокировки: %d, ожидали %d", got, before.UsersBlocked)
	}
}

// TestPostgresAdminOverviewRoleAndPaymentAggregates — остальные срезы сводки на
// конкретных сущностях: роли, курсы по статусам, успешные платежи (суммы и
// 30-дневное окно), записи на курсы.
func TestPostgresAdminOverviewRoleAndPaymentAggregates(t *testing.T) {
	s := testStore(t)
	before := s.AdminOverview()

	teacher := createUser(t, s, "ov-teacher", repo.RoleTeacher)
	createUser(t, s, "ov-admin", repo.RoleAdmin)
	course := createCourse(t, s, teacher.ID, "ov-course", 10000)
	student := createUser(t, s, "ov-student", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)
	payment := createPayment(t, s, student.ID, course.ID, 10000)
	if !s.SetPaymentStatus(payment.ID, repo.PaymentSucceeded) {
		t.Fatalf("SetPaymentStatus(succeeded) = false")
	}

	got := s.AdminOverview()
	if got.UsersTeachers != before.UsersTeachers+1 {
		t.Fatalf("UsersTeachers: было %d, стало %d, ожидали +1", before.UsersTeachers, got.UsersTeachers)
	}
	if got.UsersAdmins != before.UsersAdmins+1 {
		t.Fatalf("UsersAdmins: было %d, стало %d, ожидали +1", before.UsersAdmins, got.UsersAdmins)
	}
	if got.CoursesTotal != before.CoursesTotal+1 {
		t.Fatalf("CoursesTotal: было %d, стало %d, ожидали +1", before.CoursesTotal, got.CoursesTotal)
	}
	if got.CoursesDraft != before.CoursesDraft+1 {
		t.Fatalf("CoursesDraft: было %d, стало %d, ожидали +1", before.CoursesDraft, got.CoursesDraft)
	}
	if got.PaymentsCount != before.PaymentsCount+1 {
		t.Fatalf("PaymentsCount: было %d, стало %d, ожидали +1", before.PaymentsCount, got.PaymentsCount)
	}
	if got.PaidCents != before.PaidCents+10000 {
		t.Fatalf("PaidCents: было %d, стало %d, ожидали +10000", before.PaidCents, got.PaidCents)
	}
	if got.PlatformFeeCents != before.PlatformFeeCents+1500 {
		t.Fatalf("PlatformFeeCents: было %d, стало %d, ожидали +1500", before.PlatformFeeCents, got.PlatformFeeCents)
	}
	if got.TeacherCents != before.TeacherCents+8500 {
		t.Fatalf("TeacherCents: было %d, стало %d, ожидали +8500", before.TeacherCents, got.TeacherCents)
	}
	if got.PaidLast30Cents != before.PaidLast30Cents+10000 {
		t.Fatalf("PaidLast30Cents: было %d, стало %d, ожидали +10000", before.PaidLast30Cents, got.PaidLast30Cents)
	}
	if got.EnrollmentsTotal != before.EnrollmentsTotal+1 {
		t.Fatalf("EnrollmentsTotal: было %d, стало %d, ожидали +1", before.EnrollmentsTotal, got.EnrollmentsTotal)
	}
	if got.EnrollmentsActive != before.EnrollmentsActive+1 {
		t.Fatalf("EnrollmentsActive: было %d, стало %d, ожидали +1", before.EnrollmentsActive, got.EnrollmentsActive)
	}
}
