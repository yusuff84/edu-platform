package postgres_test

// T34 — интеграционные тесты SQL-ветки хранилища (repo.PostgresStore) на
// настоящем Postgres.
//
// Все тесты env-gated: без TEST_DATABASE_URL каждый тест делает t.Skip с
// понятным текстом, поэтому `go test ./...` на машине без Postgres остаётся
// зелёным (пакет — ok, тесты — SKIP). С заданным TEST_DATABASE_URL помощник
// поднимает pgxpool, применяет миграции (internal/migrate — как cmd/api)
// и возвращает *repo.PostgresStore.
//
// Тесты безопасны на живом стенде: работают только со своими сущностями
// (префикс pgtest-<unix-секунды> в email/slug/provider_payment_id), не делают
// TRUNCATE и не удаляют данные, которые не создали сами; агрегаты проверяются
// дельтами и выборками, отфильтрованными по своим данным.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"edu-platform/backend/internal/migrate"
	"edu-platform/backend/internal/repo"
)

// runPrefix — префикс всех данных прогона. Уникален для процесса: разные
// прогоны (и параллельные агенты) не мешают друг другу, а на живом стенде
// префикс сразу показывает, что данные — тестовые.
var runPrefix = fmt.Sprintf("pgtest-%d", time.Now().Unix())

var (
	initOnce sync.Once
	initPool *pgxpool.Pool
	initErr  error
)

// testStore поднимает пул из TEST_DATABASE_URL, применяет миграции
// (идемпотентный раннер internal/migrate — тот же, что в cmd/api) и
// возвращает PostgresStore. Без TEST_DATABASE_URL — t.Skip.
func testStore(t *testing.T) *repo.PostgresStore {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: интеграционные тесты Postgres пропущены (см. README, раздел «Интеграционные тесты Postgres»)")
	}
	initOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		initPool, initErr = repo.ConnectPool(ctx, dsn)
		if initErr != nil {
			return
		}
		if initErr = initPool.Ping(ctx); initErr != nil {
			return
		}
		var dir string
		// Рабочий каталог тестового бинарника — каталог пакета
		// backend/tests/postgres (go test ./tests/postgres/... из backend).
		dir, initErr = migrate.FindMigrationsDir(
			"../../internal/migrate/migrations",
			"internal/migrate/migrations",
		)
		if initErr != nil {
			return
		}
		initErr = migrate.ApplyMigrations(ctx, initPool, dir)
	})
	if initErr != nil {
		t.Fatalf("не удалось подготовить Postgres: %v", initErr)
	}
	return repo.NewPostgresStore(initPool, 15)
}

// shortID — 8 hex-символов UUID: уникальный суффикс данных внутри прогона.
func shortID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
}

// uniqueEmail — email с префиксом прогона: pgtest-<unix>-<tag>-<id>@example.test.
func uniqueEmail(tag string) string {
	return fmt.Sprintf("%s-%s-%s@example.test", runPrefix, tag, shortID())
}

// uniqueSlug — slug курса с префиксом прогона.
func uniqueSlug(tag string) string {
	return fmt.Sprintf("%s-%s-%s", runPrefix, tag, shortID())
}

// dummyHash — заглушка bcrypt-хеша: колонка NOT NULL, содержимое не важно —
// тесты работают с хранилищем напрямую, вход не выполняется.
const dummyHash = "test-hash-not-used-for-login"

// createUser создаёт своего пользователя и проверяет, что он реально в БД.
func createUser(t *testing.T, s *repo.PostgresStore, tag string, role repo.Role) *repo.User {
	t.Helper()
	email := uniqueEmail(tag)
	u, err := s.CreateUser(email, dummyHash, role)
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", email, err)
	}
	got, ok := s.GetUserByID(u.ID)
	if !ok || got.Email != email || got.Role != role || got.Status != repo.UserActive {
		t.Fatalf("пользователь %s не сохранён в БД: ok=%v got=%+v", email, ok, got)
	}
	return got
}

// createCourse создаёт курс-черновик своего преподавателя и проверяет запись.
func createCourse(t *testing.T, s *repo.PostgresStore, teacherID, tag string, priceCents int) *repo.Course {
	t.Helper()
	c := s.CreateCourse(&repo.Course{
		TeacherID:   teacherID,
		Slug:        uniqueSlug(tag),
		Title:       fmt.Sprintf("Курс T34 (%s-%s)", runPrefix, tag),
		Description: "Курс интеграционного теста T34",
		PriceCents:  priceCents,
		Currency:    "RUB",
	})
	got, ok := s.GetCourseByID(c.ID)
	if !ok || got.Slug != c.Slug {
		t.Fatalf("курс %s не сохранён в БД: ok=%v got=%+v", c.Slug, ok, got)
	}
	return got
}

// seedCourseWithLessons создаёт преподавателя, его курс, один модуль и n
// уроков; возвращает уроки в порядке программы (позиции 1..n).
func seedCourseWithLessons(t *testing.T, s *repo.PostgresStore, n int) (*repo.User, *repo.Course, []string) {
	t.Helper()
	teacher := createUser(t, s, "teacher", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "course", 490000)
	m := s.CreateModule(course.ID, "Модуль T34")
	if got, ok := s.GetModule(m.ID); !ok || got.CourseID != course.ID {
		t.Fatalf("модуль %s не сохранён в БД: ok=%v got=%+v", m.ID, ok, got)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		l := s.CreateLesson(&repo.Lesson{
			ModuleID: m.ID,
			Title:    fmt.Sprintf("Урок T34 %d", i+1),
			Kind:     repo.LessonVideoText,
			BodyMD:   "текст урока",
		})
		if l.ID == "" {
			t.Fatalf("CreateLesson #%d не вернул id", i+1)
		}
		ids = append(ids, l.ID)
	}
	got := s.ListLessonsByCourse(course.ID)
	if len(got) != n {
		t.Fatalf("уроков курса в БД: %d, ожидали %d", len(got), n)
	}
	for i := range got {
		if got[i].ID != ids[i] {
			t.Fatalf("порядок уроков нарушен: got[%d]=%s, ожидали %s", i, got[i].ID, ids[i])
		}
	}
	return teacher, course, ids
}

// enroll записывает ученика на курс и проверяет запись в БД.
func enroll(t *testing.T, s *repo.PostgresStore, studentID, courseID string) *repo.Enrollment {
	t.Helper()
	s.EnsureEnrollment(studentID, courseID)
	got, ok := s.GetEnrollment(studentID, courseID)
	if !ok || got.Status != repo.EnrollmentActive {
		t.Fatalf("запись ученика %s на курс %s не создана: ok=%v got=%+v", studentID, courseID, ok, got)
	}
	return got
}

// createPayment создаёт платёж в статусе pending (суммы сплита — комиссия
// 15%, как в проде) и проверяет запись в БД. Статус меняется SetPaymentStatus,
// как на реальном пути оплаты.
func createPayment(t *testing.T, s *repo.PostgresStore, studentID, courseID string, amountCents int) *repo.Payment {
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
		ProviderPaymentID:  runPrefix + "-pay-" + shortID(),
		Status:             repo.PaymentPending,
	})
	if err != nil {
		t.Fatalf("CreatePayment(%d): %v", amountCents, err)
	}
	if p.ID == "" || p.Status != repo.PaymentPending {
		t.Fatalf("платёж не сохранён: %+v", p)
	}
	return p
}

// percent — та же арифметика, что studentPercent в handler T22:
// целочисленное деление, 0 при отсутствии уроков.
func percent(completed, total int) int {
	if total <= 0 {
		return 0
	}
	return completed * 100 / total
}

// nextLessonID — та же логика, что GET /me/courses/{id}/progress (T23):
// первый непройденный урок в порядке программы или nil.
func nextLessonID(s repo.StoreAPI, studentID, courseID string) *string {
	passed := make(map[string]bool)
	for _, id := range s.ListCompletedLessonIDs(studentID, courseID) {
		passed[id] = true
	}
	for _, l := range s.ListLessonsByCourse(courseID) {
		if !passed[l.ID] {
			id := l.ID
			return &id
		}
	}
	return nil
}

// sameMicro сравнивает моменты с точностью до микросекунды: Postgres хранит
// timestamptz с микросекундной точностью и «обрезает» наносекунды Go-времени.
func sameMicro(a, b time.Time) bool {
	return a.UnixMicro() == b.UnixMicro()
}
