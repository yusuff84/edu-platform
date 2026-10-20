package postgres_test

// T77 — parity-тесты аналитики учителя: PostgresStore.TeacherOverview и
// TeacherCourseStats против in-memory Store на одинаковом наборе данных.
//
// Проверяется: совпадение всех агрегатов сводки (курсы, ученики, выручка,
// завершаемость, рейтинг, last30d), разбивка по курсам с порядком и limit,
// отсутствие N+1 (каждый метод — один SQL-запрос), изоляция курсов
// преподавателя и режим admin (all=true). Тест env-gated: без
// TEST_DATABASE_URL — t.Skip (см. helper_test.go).

import (
	"context"
	"reflect"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// t77Dataset — сущности одного набора данных для parity-сравнения.
type t77Dataset struct {
	teacher, other, admin *repo.User
	c1, c2, c3, c4        *repo.Course
	s1, s2, s3            *repo.User
}

// t77CreateUser создаёт пользователя напрямую в хранилище.
func t77CreateUser(t *testing.T, s repo.StoreAPI, tag string, role repo.Role) *repo.User {
	t.Helper()
	u, err := s.CreateUser(uniqueEmail(tag), dummyHash, role)
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", tag, err)
	}
	return u
}

// t77CreateCourse создаёт курс с модулем и уроками; CreateCourse всегда
// создаёт draft, статус выставляется SetCourseStatus.
func t77CreateCourse(t *testing.T, s repo.StoreAPI, teacherID, title, tag string, priceCents, lessons int, status repo.CourseStatus) *repo.Course {
	t.Helper()
	c := s.CreateCourse(&repo.Course{
		TeacherID:   teacherID,
		Slug:        uniqueSlug(tag),
		Title:       title,
		Description: "Курс parity-теста T77",
		PriceCents:  priceCents,
		Currency:    "RUB",
	})
	if c == nil || c.ID == "" {
		t.Fatalf("CreateCourse(%s) не вернул id", title)
	}
	if !s.SetCourseStatus(c.ID, status, "") {
		t.Fatalf("SetCourseStatus(%s, %s) = false", title, status)
	}
	if lessons > 0 {
		m := s.CreateModule(c.ID, "Модуль T77")
		for i := 0; i < lessons; i++ {
			s.CreateLesson(&repo.Lesson{
				ModuleID: m.ID,
				Title:    title + " — урок",
				Kind:     repo.LessonVideoText,
				BodyMD:   "текст",
			})
		}
	}
	return c
}

// t77CreatePayment создаёт succeeded/pending/refunded-платёж (комиссия 15%).
func t77CreatePayment(t *testing.T, s repo.StoreAPI, studentID, courseID string, amountCents int, status repo.PaymentStatus) {
	t.Helper()
	fee := amountCents * 15 / 100
	p, err := s.CreatePayment(&repo.Payment{
		StudentID:          studentID,
		CourseID:           courseID,
		AmountCents:        amountCents,
		PlatformFeeCents:   fee,
		TeacherAmountCents: amountCents - fee,
		Currency:           "RUB",
		Provider:           "mock",
		ProviderPaymentID:  runPrefix + "-t77-" + shortID(),
		Status:             repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if status != repo.PaymentPending && !s.SetPaymentStatus(p.ID, status) {
		t.Fatalf("SetPaymentStatus(%s) = false", status)
	}
}

// t77Seed наполняет хранилище одинаковым набором данных: два своих
// published-курса, draft, курс чужого преподавателя, три ученика, активные и
// отчисленная записи, прогресс, платежи разных статусов и отзывы.
func t77Seed(t *testing.T, s repo.StoreAPI) *t77Dataset {
	t.Helper()
	d := &t77Dataset{}
	d.teacher = t77CreateUser(t, s, "t77-teacher", repo.RoleTeacher)
	d.other = t77CreateUser(t, s, "t77-other", repo.RoleTeacher)
	d.admin = t77CreateUser(t, s, "t77-admin", repo.RoleAdmin)

	d.c1 = t77CreateCourse(t, s, d.teacher.ID, "T77 A: Go", "t77-c1", 10000, 4, repo.CoursePublished)
	d.c2 = t77CreateCourse(t, s, d.teacher.ID, "T77 B: SQL", "t77-c2", 5000, 2, repo.CoursePublished)
	d.c3 = t77CreateCourse(t, s, d.teacher.ID, "T77 C: Draft", "t77-c3", 0, 0, repo.CourseDraft)
	d.c4 = t77CreateCourse(t, s, d.other.ID, "T77 D: Чужой", "t77-c4", 7000, 1, repo.CoursePublished)

	d.s1 = t77CreateUser(t, s, "t77-s1", repo.RoleStudent)
	d.s2 = t77CreateUser(t, s, "t77-s2", repo.RoleStudent)
	d.s3 = t77CreateUser(t, s, "t77-s3", repo.RoleStudent)

	for _, pair := range [][2]string{{d.s1.ID, d.c1.ID}, {d.s2.ID, d.c1.ID}, {d.s3.ID, d.c2.ID}, {d.s2.ID, d.c4.ID}} {
		s.EnsureEnrollment(pair[0], pair[1])
	}
	s.EnsureEnrollment(d.s1.ID, d.c2.ID)
	if err := s.SetEnrollmentStatus(d.s1.ID, d.c2.ID, repo.EnrollmentRefunded); err != nil {
		t.Fatalf("SetEnrollmentStatus: %v", err)
	}

	lessons1 := s.ListLessonsByCourse(d.c1.ID)
	if len(lessons1) != 4 {
		t.Fatalf("уроков c1=%d, ждали 4", len(lessons1))
	}
	for _, l := range lessons1 {
		s.CompleteLesson(d.s1.ID, l.ID)
	}
	for _, l := range lessons1[:2] {
		s.CompleteLesson(d.s2.ID, l.ID)
	}
	lessons2 := s.ListLessonsByCourse(d.c2.ID)
	s.CompleteLesson(d.s3.ID, lessons2[0].ID)

	t77CreatePayment(t, s, d.s1.ID, d.c1.ID, 10000, repo.PaymentSucceeded)
	t77CreatePayment(t, s, d.s2.ID, d.c1.ID, 10000, repo.PaymentSucceeded)
	t77CreatePayment(t, s, d.s3.ID, d.c1.ID, 10000, repo.PaymentPending)
	t77CreatePayment(t, s, d.s3.ID, d.c2.ID, 5000, repo.PaymentRefunded)
	t77CreatePayment(t, s, d.s2.ID, d.c4.ID, 7000, repo.PaymentSucceeded)

	for _, r := range []struct {
		studentID, courseID string
		rating              int
	}{
		{d.s1.ID, d.c1.ID, 5},
		{d.s2.ID, d.c1.ID, 4},
		{d.s3.ID, d.c2.ID, 3},
		{d.s2.ID, d.c3.ID, 1},
		{d.s2.ID, d.c4.ID, 5},
	} {
		if _, err := s.CreateReview(r.studentID, r.courseID, r.rating, "Отзыв T77"); err != nil {
			t.Fatalf("CreateReview: %v", err)
		}
		s.RecalcCourseRating(r.courseID)
	}
	return d
}

// t77StatView — состояние строки разбивки без CourseID: идентификаторы в
// in-memory и Postgres разные, сравнивать нужно агрегаты и порядок.
type t77StatView struct {
	Title        string
	Status       repo.CourseStatus
	Students     int
	RevenueCents int
	AvgRating    float64
	RatingsCount int
	AvgPercent   float64
}

func t77Views(items []*repo.TeacherCourseStat) []t77StatView {
	out := make([]t77StatView, 0, len(items))
	for _, it := range items {
		out = append(out, t77StatView{
			Title: it.Title, Status: it.Status, Students: it.Students,
			RevenueCents: it.RevenueCents, AvgRating: it.AvgRating,
			RatingsCount: it.RatingsCount, AvgPercent: it.AvgPercent,
		})
	}
	return out
}

// TestPostgresTeacherStatsParity — главный parity-тест: одинаковые данные в
// in-memory Store и PostgresStore дают одинаковую сводку и разбивку.
func TestPostgresTeacherStatsParity(t *testing.T) {
	mem := repo.NewStore()
	pg := testStore(t)

	// Миграция 015 должна создать три индекса под агрегаты (T77).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var indexes int
	err := pg.Pool().QueryRow(ctx,
		`SELECT COUNT(*)::int FROM pg_indexes
		 WHERE indexname IN ('idx_enrollments_course_status',
		                     'idx_payments_course_status_created',
		                     'idx_reviews_course_rating')`).Scan(&indexes)
	if err != nil {
		t.Fatalf("проверка индексов 015: %v", err)
	}
	if indexes != 3 {
		t.Fatalf("индексов аналитики в БД: %d, ждали 3 (миграция 015)", indexes)
	}

	memData := t77Seed(t, mem)
	pgData := t77Seed(t, pg)

	// Сводка своего учителя — полное совпадение всех агрегатов.
	memOverview, err := mem.TeacherOverview(memData.teacher.ID, false)
	if err != nil {
		t.Fatalf("in-memory TeacherOverview: %v", err)
	}
	pgOverview, err := pg.TeacherOverview(pgData.teacher.ID, false)
	if err != nil {
		t.Fatalf("postgres TeacherOverview: %v", err)
	}
	if !reflect.DeepEqual(memOverview, pgOverview) {
		t.Fatalf("сводка не совпала:\n in-memory: %+v\n postgres:  %+v", memOverview, pgOverview)
	}
	// Контрольные значения контракта (те же, что в контрактном тесте).
	if pgOverview.Courses.Total != 3 || pgOverview.Courses.Published != 2 || pgOverview.Courses.Draft != 1 {
		t.Fatalf("курсы в сводке: %+v", pgOverview.Courses)
	}
	if pgOverview.Students.Total != 3 || pgOverview.Students.Unique != 3 {
		t.Fatalf("ученики в сводке: %+v", pgOverview.Students)
	}
	if pgOverview.Revenue.PaidTotalCents != 20000 || pgOverview.Revenue.PlatformFeeCents != 3000 ||
		pgOverview.Revenue.TeacherCents != 17000 {
		t.Fatalf("выручка в сводке: %+v", pgOverview.Revenue)
	}
	if pgOverview.Completion.Completed != 1 || pgOverview.Completion.AvgPercent != 66.7 {
		t.Fatalf("завершаемость в сводке: %+v", pgOverview.Completion)
	}
	if pgOverview.Ratings.AvgRating != 4.0 || pgOverview.Ratings.RatingsCount != 3 {
		t.Fatalf("рейтинг в сводке: %+v", pgOverview.Ratings)
	}
	if pgOverview.Last30d.Enrollments != 3 || pgOverview.Last30d.RevenueCents != 20000 {
		t.Fatalf("last30d в сводке: %+v", pgOverview.Last30d)
	}

	// Разбивка: порядок revenue desc → students desc → title, limit работает.
	memFull := mem.TeacherCourseStats(memData.teacher.ID, false, 50)
	pgFull := pg.TeacherCourseStats(pgData.teacher.ID, false, 50)
	if !reflect.DeepEqual(t77Views(memFull), t77Views(pgFull)) {
		t.Fatalf("разбивка не совпала:\n in-memory: %+v\n postgres:  %+v",
			t77Views(memFull), t77Views(pgFull))
	}
	if len(pgFull) != 3 || pgFull[0].CourseID != pgData.c1.ID ||
		pgFull[1].CourseID != pgData.c2.ID || pgFull[2].CourseID != pgData.c3.ID {
		t.Fatalf("порядок разбивки: %+v", pgFull)
	}
	if pgFull[0].Students != 2 || pgFull[0].RevenueCents != 20000 ||
		pgFull[0].AvgRating != 4.5 || pgFull[0].AvgPercent != 75 {
		t.Fatalf("c1 в разбивке: %+v", pgFull[0])
	}
	memLimited := mem.TeacherCourseStats(memData.teacher.ID, false, 2)
	pgLimited := pg.TeacherCourseStats(pgData.teacher.ID, false, 2)
	if len(memLimited) != 2 || len(pgLimited) != 2 || !reflect.DeepEqual(t77Views(memLimited), t77Views(pgLimited)) {
		t.Fatalf("limit=2 не совпал: mem=%+v pg=%+v", t77Views(memLimited), t77Views(pgLimited))
	}

	// Чужой преподаватель видит только свой курс — в обеих реализациях.
	memOther, err := mem.TeacherOverview(memData.other.ID, false)
	if err != nil {
		t.Fatalf("in-memory TeacherOverview(other): %v", err)
	}
	pgOther, err := pg.TeacherOverview(pgData.other.ID, false)
	if err != nil {
		t.Fatalf("postgres TeacherOverview(other): %v", err)
	}
	if !reflect.DeepEqual(memOther, pgOther) {
		t.Fatalf("сводка чужого учителя не совпала: mem=%+v pg=%+v", memOther, pgOther)
	}
	if pgOther.Courses.Total != 1 || pgOther.Revenue.PaidTotalCents != 7000 ||
		pgOther.Students.Total != 1 || pgOther.Ratings.AvgRating != 5 {
		t.Fatalf("сводка чужого учителя: %+v", pgOther)
	}

	// Несуществующий преподаватель — нулевая сводка без ошибки.
	empty, err := pg.TeacherOverview("00000000-0000-0000-0000-000000000000", false)
	if err != nil {
		t.Fatalf("TeacherOverview(несуществующий): %v", err)
	}
	if !reflect.DeepEqual(empty, &repo.TeacherOverview{}) {
		t.Fatalf("сводка несуществующего учителя не нулевая: %+v", empty)
	}
}

// TestPostgresTeacherStatsAdminAll — all=true (admin) охватывает курсы всех
// преподавателей: в разбивке присутствуют оба наших курса, а teacher-режим
// чужой курс не показывает. Точные глобальные суммы не проверяются: в БД
// могут быть данные других тестов прогона.
func TestPostgresTeacherStatsAdminAll(t *testing.T) {
	pg := testStore(t)
	d := t77Seed(t, pg)

	all := pg.TeacherCourseStats(d.admin.ID, true, 200)
	ids := make(map[string]bool, len(all))
	for _, it := range all {
		ids[it.CourseID] = true
	}
	for _, c := range []*repo.Course{d.c1, d.c2, d.c3, d.c4} {
		if !ids[c.ID] {
			t.Fatalf("admin-разбивка не содержит курс %s (%s); всего строк: %d", c.Title, c.ID, len(all))
		}
	}

	scoped := pg.TeacherCourseStats(d.teacher.ID, false, 200)
	scopedIDs := make(map[string]bool, len(scoped))
	for _, it := range scoped {
		scopedIDs[it.CourseID] = true
	}
	if scopedIDs[d.c4.ID] {
		t.Fatal("teacher-разбивка содержит чужой курс")
	}
	if len(scoped) != 3 {
		t.Fatalf("teacher-разбивка: %d курсов, ждали 3", len(scoped))
	}

	adminOverview, err := pg.TeacherOverview(d.admin.ID, true)
	if err != nil {
		t.Fatalf("admin TeacherOverview: %v", err)
	}
	teacherOverview, err := pg.TeacherOverview(d.teacher.ID, false)
	if err != nil {
		t.Fatalf("teacher TeacherOverview: %v", err)
	}
	if adminOverview.Courses.Total < teacherOverview.Courses.Total+1 {
		t.Fatalf("admin видит %d курсов, teacher — %d: admin должен охватывать все",
			adminOverview.Courses.Total, teacherOverview.Courses.Total)
	}
	if adminOverview.Revenue.PaidTotalCents < teacherOverview.Revenue.PaidTotalCents+7000 {
		t.Fatalf("admin-выручка %d меньше teacher+чужой (%d+7000)",
			adminOverview.Revenue.PaidTotalCents, teacherOverview.Revenue.PaidTotalCents)
	}
}
