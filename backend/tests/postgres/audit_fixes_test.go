package postgres_test

// Регрессионные тесты исправлений аудита бэкенда (ноябрь 2026) на живом
// Postgres и паритет с in-memory хранилищем.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/repo"
)

const hugePage = 9223372036854775807

// Смена email гасит выданные письмами токены; ClaimEmailOwnership
// подтверждает адрес, меняет пароль и завершает сессии и токены.
func TestAuditAccountClaimParity(t *testing.T) {
	t.Run("memory", func(t *testing.T) { testAuditAccountClaim(t, repo.NewStore()) })
	t.Run("postgres", func(t *testing.T) { testAuditAccountClaim(t, testStore(t)) })
}

func testAuditAccountClaim(t *testing.T, s repo.StoreAPI) {
	now := time.Now().UTC()
	u, err := s.CreateUser(uniqueEmail("audit-claim"), "attacker-hash", repo.RoleStudent)
	if err != nil {
		t.Fatal(err)
	}

	// 1. SetUserEmail: старая ссылка подтверждения больше не работает.
	_, verifyHash, _, _ := auth.GenerateToken()
	if err := s.IssueAccountToken(u.ID, "attacker-hash", repo.EmailVerification, verifyHash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserEmail(u.ID, uniqueEmail("audit-claim-new")); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeAccountToken(verifyHash, repo.EmailVerification, "", now); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatalf("токен после смены email: %v", err)
	}
	if fresh, _ := s.GetUserByID(u.ID); fresh.EmailVerifiedAt != nil {
		t.Fatal("новый адрес подтверждён старой ссылкой")
	}

	// 2. ClaimEmailOwnership.
	sess, err := s.CreateAccountSession(u.ID, "attacker-hash", time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, resetHash, _, _ := auth.GenerateToken()
	if err := s.IssueAccountToken(u.ID, "attacker-hash", repo.PasswordReset, resetHash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimEmailOwnership(u.ID, "owner-hash"); err != nil {
		t.Fatal(err)
	}
	fresh, _ := s.GetUserByID(u.ID)
	if fresh.PasswordHash != "owner-hash" || fresh.EmailVerifiedAt == nil {
		t.Fatalf("после claim: hash=%q verified=%v", fresh.PasswordHash, fresh.EmailVerifiedAt)
	}
	if _, ok := s.GetSession(sess.ID); ok {
		t.Fatal("сессия регистратора пережила claim")
	}
	if err := s.ConsumeAccountToken(resetHash, repo.PasswordReset, "x", now); !errors.Is(err, repo.ErrInvalidAccountToken) {
		t.Fatalf("ссылка сброса пароля пережила claim: %v", err)
	}
	verifiedAt := *fresh.EmailVerifiedAt
	if err := s.ClaimEmailOwnership(u.ID, "owner-hash"); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.GetUserByID(u.ID); !again.EmailVerifiedAt.Equal(verifiedAt) {
		t.Fatal("повторный claim не должен сдвигать момент подтверждения")
	}

	// Заблокированный и несуществующий — ErrNotFound.
	if !s.SetUserStatus(u.ID, repo.UserBlocked) {
		t.Fatal("SetUserStatus")
	}
	if err := s.ClaimEmailOwnership(u.ID, "h"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("claim заблокированного: %v", err)
	}
	if err := s.ClaimEmailOwnership(uuid.NewString(), "h"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("claim несуществующего: %v", err)
	}
}

// Ошибка БД при создании курса/модуля/урока — nil, а не «объект» со
// случайным id, которого нет в базе.
func TestAuditPostgresCreateFailuresReturnNil(t *testing.T) {
	s := testStore(t)
	if c := s.CreateCourse(&repo.Course{TeacherID: uuid.NewString(), Slug: uniqueSlug("orphan"), Title: "x", Description: "x"}); c != nil {
		t.Fatalf("курс без преподавателя создан: %+v", c)
	}
	if m := s.CreateModule(uuid.NewString(), "x"); m != nil {
		t.Fatalf("модуль без курса создан: %+v", m)
	}
	if l := s.CreateLesson(&repo.Lesson{ModuleID: uuid.NewString(), Title: "x", Kind: repo.LessonVideoText, BodyMD: "x"}); l != nil {
		t.Fatalf("урок без модуля создан: %+v", l)
	}
	// Цена вне INT4 — тоже ошибка БД, а не фиктивный курс.
	teacher := createUser(t, s, "orphan-teacher", repo.RoleTeacher)
	if c := s.CreateCourse(&repo.Course{TeacherID: teacher.ID, Slug: uniqueSlug("int4"), Title: "x", Description: "x", PriceCents: 3_000_000_000}); c != nil {
		t.Fatalf("курс с ценой вне INT4 создан: %+v", c)
	}
}

// Огромный page — пустая страница с честным total, а не ошибка OFFSET.
func TestAuditPostgresHugePage(t *testing.T) {
	s := testStore(t)
	_, course, lessons := seedCourseWithLessons(t, s, 1)
	student := createUser(t, s, "huge-page-student", repo.RoleStudent)
	if _, err := s.CreateLessonQuestion(course.ID, lessons[0], student.ID, "Вопрос для проверки пагинации"); err != nil {
		t.Fatal(err)
	}
	items, total := s.ListLessonQuestions(course.ID, "", hugePage, 20)
	if len(items) != 0 || total != 1 {
		t.Fatalf("ListLessonQuestions: items=%d total=%d", len(items), total)
	}
	s.AddAuditEntry("", "audit@example.com", "проверка пагинации", "")
	if items, total := s.ListAuditEntries("", hugePage, 30); len(items) != 0 || total == 0 {
		t.Fatalf("ListAuditEntries: items=%d total=%d", len(items), total)
	}
	if rows, total := s.ListAllCourses(repo.CourseFilter{Page: hugePage, Limit: 100}); len(rows) != 0 || total == 0 {
		t.Fatalf("ListAllCourses: rows=%d total=%d", len(rows), total)
	}
	if rows, total, _, err := s.ListPaymentsJournal(repo.PaymentFilter{Page: hugePage, Limit: 100}); err != nil || len(rows) != 0 {
		t.Fatalf("ListPaymentsJournal: rows=%d total=%d err=%v", len(rows), total, err)
	}
}

// Миграция 021 создаёт индексы под горячие запросы.
func TestAuditHotPathIndexes(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{
		"idx_users_email_lower", "idx_courses_slug_lower", "idx_courses_teacher_created",
		"idx_payments_student_course", "idx_lesson_progress_lesson",
	} {
		var n int
		if err := s.Pool().QueryRow(context.Background(),
			`SELECT count(*) FROM pg_indexes WHERE indexname=$1`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("индекс %s: n=%d err=%v", name, n, err)
		}
	}
}
