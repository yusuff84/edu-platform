package postgres_test

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestPostgresStudentDashboard — данные кабинета ученика (T23) на уровне
// репозитория: ListEnrollmentsByStudent, ListPaymentsByStudent и прогресс
// (completedLessonIds/nextLessonId — как в handler MyCourseProgress).
func TestPostgresStudentDashboard(t *testing.T) {
	s := testStore(t)

	_, course, lessons := seedCourseWithLessons(t, s, 3)
	student := createUser(t, s, "dash-student", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)

	// Записи ученика: одна активная на свой курс.
	enrs := s.ListEnrollmentsByStudent(student.ID)
	if len(enrs) != 1 || enrs[0].CourseID != course.ID || enrs[0].Status != repo.EnrollmentActive {
		t.Fatalf("ListEnrollmentsByStudent: %d записей, ожидали 1 активную на %s", len(enrs), course.ID)
	}

	// Платежи ученика: pending и succeeded видны, новые первыми.
	pPending := createPayment(t, s, student.ID, course.ID, 1000)
	pOk := createPayment(t, s, student.ID, course.ID, 2000)
	if !s.SetPaymentStatus(pOk.ID, repo.PaymentSucceeded) {
		t.Fatalf("SetPaymentStatus(succeeded) = false")
	}
	pays := s.ListPaymentsByStudent(student.ID)
	if len(pays) != 2 {
		t.Fatalf("ListPaymentsByStudent: %d платежей, ожидали 2", len(pays))
	}
	if pays[0].ID != pOk.ID || pays[0].AmountCents != 2000 ||
		pays[0].Status != repo.PaymentSucceeded || pays[0].ProcessedAt == nil {
		t.Fatalf("первый платёж: %+v, ожидали succeeded 2000 с processed_at", pays[0])
	}
	if pays[1].ID != pPending.ID || pays[1].Status != repo.PaymentPending || pays[1].ProcessedAt != nil {
		t.Fatalf("второй платёж: %+v, ожидали pending 1000 без processed_at", pays[1])
	}
	// У другого ученика своих платежей нет.
	other := createUser(t, s, "dash-other", repo.RoleStudent)
	if got := s.ListPaymentsByStudent(other.ID); len(got) != 0 {
		t.Fatalf("у непричастного ученика %d платежей, ожидали 0", len(got))
	}

	// Прогресс до старта: completedLessonIds — пустой (не nil), следующий — урок 1.
	done := s.ListCompletedLessonIDs(student.ID, course.ID)
	if done == nil || len(done) != 0 {
		t.Fatalf("ListCompletedLessonIDs до старта: %#v, ожидали пустой не-nil", done)
	}
	if got := nextLessonID(s, student.ID, course.ID); got == nil || *got != lessons[0] {
		t.Fatalf("nextLessonId до старта: %v, ожидали урок 1 (%s)", got, lessons[0])
	}

	// Пропуск: второй ученик проходит только урок 2 — в списке он один,
	// следующий всё равно урок 1 (первый непройденный по программе).
	skipper := createUser(t, s, "dash-skipper", repo.RoleStudent)
	enroll(t, s, skipper.ID, course.ID)
	s.CompleteLesson(skipper.ID, lessons[1])
	doneSkip := s.ListCompletedLessonIDs(skipper.ID, course.ID)
	if len(doneSkip) != 1 || doneSkip[0] != lessons[1] {
		t.Fatalf("completedLessonIds пропустившего (урок 2): %v, ожидали [%s]", doneSkip, lessons[1])
	}
	if got := nextLessonID(s, skipper.ID, course.ID); got == nil || *got != lessons[0] {
		t.Fatalf("nextLessonId пропустившего (урок 2): %v, ожидали урок 1", got)
	}

	// Уроки 1 и 3: порядок по программе [1, 3], следующий — урок 2.
	s.CompleteLesson(student.ID, lessons[0])
	s.CompleteLesson(student.ID, lessons[2])
	done = s.ListCompletedLessonIDs(student.ID, course.ID)
	if len(done) != 2 || done[0] != lessons[0] || done[1] != lessons[2] {
		t.Fatalf("completedLessonIds после уроков 1 и 3: %v, ожидали [1, 3]", done)
	}
	if got := nextLessonID(s, student.ID, course.ID); got == nil || *got != lessons[1] {
		t.Fatalf("nextLessonId после уроков 1 и 3: %v, ожидали урок 2", got)
	}

	// Полное прохождение: список [1, 2, 3], nextLessonId — nil, percent 100.
	s.CompleteLesson(student.ID, lessons[1])
	done = s.ListCompletedLessonIDs(student.ID, course.ID)
	if len(done) != 3 || done[0] != lessons[0] || done[1] != lessons[1] || done[2] != lessons[2] {
		t.Fatalf("completedLessonIds при полном прохождении: %v, ожидали [1, 2, 3]", done)
	}
	if got := s.CountCompletedLessons(student.ID, course.ID); got != 3 {
		t.Fatalf("CountCompletedLessons = %d, ожидали 3", got)
	}
	if got := percent(3, 3); got != 100 {
		t.Fatalf("percent(3, 3) = %d, ожидали 100", got)
	}
	if got := nextLessonID(s, student.ID, course.ID); got != nil {
		t.Fatalf("nextLessonId при полном прохождении: %v, ожидали nil", *got)
	}

	// Отзыв доступа: запись остаётся в списке со статусом revoked
	// (403 на прогресс выдаёт handler).
	if err := s.SetEnrollmentStatus(student.ID, course.ID, repo.EnrollmentRevoked); err != nil {
		t.Fatalf("SetEnrollmentStatus(revoked): %v", err)
	}
	enrs = s.ListEnrollmentsByStudent(student.ID)
	if len(enrs) != 1 || enrs[0].Status != repo.EnrollmentRevoked {
		t.Fatalf("после отзыва доступа: %+v, ожидали revoked", enrs)
	}
}
