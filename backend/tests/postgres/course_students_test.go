package postgres_test

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestPostgresCourseStudentsProgress — данные таба «Ученики курса» (T22):
// ListEnrollmentsByCourse отдаёт записи во всех статусах (новые первыми),
// StudentProgress — completed и lastActivity по урокам курса,
// CountLessonsByCourse/percent считаются как в handler.
func TestPostgresCourseStudentsProgress(t *testing.T) {
	s := testStore(t)

	_, course, lessons := seedCourseWithLessons(t, s, 3)
	s1 := createUser(t, s, "students-s1", repo.RoleStudent)
	s2 := createUser(t, s, "students-s2", repo.RoleStudent)
	enroll(t, s, s1.ID, course.ID)
	enroll(t, s, s2.ID, course.ID)

	// s1 проходит уроки 1 и 2; s2 «начинал» урок 1 (попытка без завершения).
	_, _ = s.CompleteLesson(s1.ID, lessons[0])
	pp2, _ := s.CompleteLesson(s1.ID, lessons[1])
	if got := s.BumpAttempt(s2.ID, lessons[0]); got != 1 {
		t.Fatalf("BumpAttempt(s2, урок 1) = %d, ожидали 1", got)
	}

	// Записи курса: обе активные, новые первыми (s2 записан вторым).
	enrs := s.ListEnrollmentsByCourse(course.ID)
	if len(enrs) != 2 || enrs[0].StudentID != s2.ID || enrs[1].StudentID != s1.ID {
		t.Fatalf("ListEnrollmentsByCourse: %d записей (%v), ожидали [s2, s1]", len(enrs), enrollmentStudents(enrs))
	}
	for _, e := range enrs {
		if e.Status != repo.EnrollmentActive {
			t.Fatalf("статус записи %s: %s, ожидали active", e.StudentID, e.Status)
		}
	}

	// Прогресс: у s1 — 2 завершённых урока, lastActivity = время второго.
	prog := s.StudentProgress(course.ID)
	sp1, ok := prog[s1.ID]
	if !ok || sp1.Completed != 2 {
		t.Fatalf("StudentProgress(s1): ok=%v %+v, ожидали Completed=2", ok, sp1)
	}
	if !sameMicro(sp1.LastActivity, pp2.CompletedAt) {
		t.Fatalf("StudentProgress(s1).LastActivity=%s, ожидали %s", sp1.LastActivity, pp2.CompletedAt)
	}
	// У s2 урок не завершён: Completed=0, lastActivity нулевая.
	sp2, ok := prog[s2.ID]
	if !ok || sp2.Completed != 0 || !sp2.LastActivity.IsZero() {
		t.Fatalf("StudentProgress(s2): ok=%v %+v, ожидали Completed=0 и нулевой LastActivity", ok, sp2)
	}

	// Не записанный на курс ученик в прогрессе отсутствует.
	s3 := createUser(t, s, "students-s3", repo.RoleStudent)
	if _, ok := s.StudentProgress(course.ID)[s3.ID]; ok {
		t.Fatalf("в прогрессе курса появился не записанный ученик %s", s3.ID)
	}

	// totalLessons и percent — как в handler T22.
	total := s.CountLessonsByCourse(course.ID)
	if total != 3 {
		t.Fatalf("CountLessonsByCourse = %d, ожидали 3", total)
	}
	if got := percent(sp1.Completed, total); got != 66 {
		t.Fatalf("percent(s1) = %d, ожидали 66 (2 из 3)", got)
	}
	if got := percent(sp2.Completed, total); got != 0 {
		t.Fatalf("percent(s2) = %d, ожидали 0", got)
	}

	// Отчисление видно учителю: запись остаётся в списке со статусом revoked.
	if err := s.SetEnrollmentStatus(s2.ID, course.ID, repo.EnrollmentRevoked); err != nil {
		t.Fatalf("SetEnrollmentStatus(revoked): %v", err)
	}
	enrs = s.ListEnrollmentsByCourse(course.ID)
	if len(enrs) != 2 || enrs[0].StudentID != s2.ID || enrs[0].Status != repo.EnrollmentRevoked {
		t.Fatalf("список записей после отчисления: %v", enrollmentStudents(enrs))
	}
}

// TestPostgresCourseStatsData — «ингредиенты» CourseStats (T22) на уровне
// репозитория: проценты по записям, выручка только по succeeded, отзывы.
// Данные собираются ровно так, как это делает handler TeacherCourseStats.
func TestPostgresCourseStatsData(t *testing.T) {
	s := testStore(t)

	_, course, lessons := seedCourseWithLessons(t, s, 3)
	s1 := createUser(t, s, "stats-s1", repo.RoleStudent)
	s2 := createUser(t, s, "stats-s2", repo.RoleStudent)
	s3 := createUser(t, s, "stats-s3", repo.RoleStudent)
	enroll(t, s, s1.ID, course.ID)
	enroll(t, s, s2.ID, course.ID)
	enroll(t, s, s3.ID, course.ID)
	if err := s.SetEnrollmentStatus(s3.ID, course.ID, repo.EnrollmentRevoked); err != nil {
		t.Fatalf("SetEnrollmentStatus(s3): %v", err)
	}

	// s1 — 100% (3 из 3), s2 — 33% (1 из 3), s3 — 0%.
	for _, lid := range lessons {
		s.CompleteLesson(s1.ID, lid)
	}
	s.CompleteLesson(s2.ID, lessons[0])

	// Платежи: succeeded 5000 идёт в выручку, failed и pending — нет.
	paid := createPayment(t, s, s1.ID, course.ID, 5000)
	if !s.SetPaymentStatus(paid.ID, repo.PaymentSucceeded) {
		t.Fatalf("SetPaymentStatus(succeeded) = false")
	}
	failed := createPayment(t, s, s2.ID, course.ID, 3000)
	if !s.SetPaymentStatus(failed.ID, repo.PaymentFailed) {
		t.Fatalf("SetPaymentStatus(failed) = false")
	}
	createPayment(t, s, s2.ID, course.ID, 1000) // остаётся pending

	// Отзыв на 5 звёзд.
	if _, err := s.CreateReview(s1.ID, course.ID, 5, "Отзыв интеграционного теста T34"); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	// Считаем статистику так же, как handler TeacherCourseStats.
	enrs := s.ListEnrollmentsByCourse(course.ID)
	prog := s.StudentProgress(course.ID)
	totalLessons := s.CountLessonsByCourse(course.ID)
	studentsTotal, studentsActive, studentsCompleted, sumPercent := 0, 0, 0, 0
	for _, e := range enrs {
		studentsTotal++
		if e.Status == repo.EnrollmentActive {
			studentsActive++
		}
		pct := percent(prog[e.StudentID].Completed, totalLessons)
		sumPercent += pct
		if totalLessons > 0 && pct == 100 {
			studentsCompleted++
		}
	}
	avgPercent := 0
	if studentsTotal > 0 {
		avgPercent = sumPercent / studentsTotal
	}
	revenueCents := 0
	for _, p := range s.ListPaymentsByCourse(course.ID) {
		if p.Status == repo.PaymentSucceeded {
			revenueCents += p.AmountCents
		}
	}
	reviews := s.ListReviewsByCourse(course.ID)

	if studentsTotal != 3 {
		t.Fatalf("studentsTotal = %d, ожидали 3", studentsTotal)
	}
	if studentsActive != 2 {
		t.Fatalf("studentsActive = %d, ожидали 2", studentsActive)
	}
	if studentsCompleted != 1 {
		t.Fatalf("studentsCompleted = %d, ожидали 1", studentsCompleted)
	}
	if avgPercent != 44 {
		t.Fatalf("avgPercent = %d, ожидали 44 ((100+33+0)/3)", avgPercent)
	}
	if revenueCents != 5000 {
		t.Fatalf("revenueCents = %d, ожидали 5000 (только succeeded)", revenueCents)
	}
	if modules := len(s.ListModulesByCourse(course.ID)); modules != 1 {
		t.Fatalf("modulesTotal = %d, ожидали 1", modules)
	}
	if totalLessons != 3 {
		t.Fatalf("lessonsTotal = %d, ожидали 3", totalLessons)
	}
	if len(reviews) != 1 || reviews[0].Rating != 5 {
		t.Fatalf("reviewsCount/avgRating: %d отзывов, ожидали 1 с рейтингом 5", len(reviews))
	}
}

// enrollmentStudents — id учеников записей для сообщений об ошибках.
func enrollmentStudents(enrs []*repo.Enrollment) []string {
	out := make([]string, 0, len(enrs))
	for _, e := range enrs {
		out = append(out, e.StudentID)
	}
	return out
}
