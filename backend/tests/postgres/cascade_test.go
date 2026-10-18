package postgres_test

// T46 — интеграционные тесты на живом Postgres: каскад удаления курса и
// идемпотентность миграций (хвост честного списка T20/T34).
//
// Правила те же, что у helper_test.go: env-gated через TEST_DATABASE_URL
// (без него — t.Skip через testStore), только свои данные с префиксом
// прогона, никаких TRUNCATE и чужих строк. Проверки «после удаления» идут
// двумя способами: публичными методами PostgresStore (как их видит API) и
// прямыми COUNT по своим id — чтобы зафиксировать фактическое поведение
// FK-каскадов 001_init.sql, а не только то, что метод вернул пусто.

import (
	"context"
	"testing"
	"time"

	"edu-platform/backend/internal/migrate"
	"edu-platform/backend/internal/repo"
)

// countRows — COUNT(*) по своему запросу к тому же пулу, что и testStore:
// прямая проверка, что строки реально удалены из БД (а не отфильтрованы
// методом). Запросы теста — только по своим id/полям.
func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var n int
	if err := initPool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("COUNT-запрос %q: %v", query, err)
	}
	return n
}

// TestPostgresDeleteCourseCascade — DeleteCourse на живом Postgres: курс с
// модулями и уроками, запись ученика, платёж (pending → succeeded) и отзыв
// уходят каскадом (FK ON DELETE CASCADE из 001_init.sql). Проверяются и
// методы хранилища, и фактические строки в таблицах; каскад не должен
// задевать самих пользователей.
func TestPostgresDeleteCourseCascade(t *testing.T) {
	s := testStore(t)

	// Свои сущности: преподаватель, курс, модуль, 2 урока.
	teacher, course, lessons := seedCourseWithLessons(t, s, 2)
	mods := s.ListModulesByCourse(course.ID)
	if len(mods) != 1 {
		t.Fatalf("модулей курса до удаления: %d, ожидали 1", len(mods))
	}

	// Ученик: запись на курс, прогресс по уроку, платёж succeeded, отзыв.
	student := createUser(t, s, "cascade-student", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)
	s.CompleteLesson(student.ID, lessons[0])
	pay := createPayment(t, s, student.ID, course.ID, 490000)
	if !s.SetPaymentStatus(pay.ID, repo.PaymentSucceeded) {
		t.Fatalf("SetPaymentStatus(succeeded, платёж %s) = false", pay.ID)
	}
	if _, err := s.CreateReview(student.ID, course.ID, 5, "Отзыв интеграционного теста T46"); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	// Санити до удаления: всё на месте и видно методами.
	if got, ok := s.GetCourseByID(course.ID); !ok || got.ID != course.ID {
		t.Fatalf("курс до удаления не читается: ok=%v got=%+v", ok, got)
	}
	if got := s.ListLessonsByCourse(course.ID); len(got) != len(lessons) {
		t.Fatalf("уроков курса до удаления: %d, ожидали %d", len(got), len(lessons))
	}
	if got, ok := s.GetEnrollment(student.ID, course.ID); !ok || got.Status != repo.EnrollmentActive {
		t.Fatalf("запись ученика до удаления: ok=%v got=%+v", ok, got)
	}
	if got := s.ListPaymentsByCourse(course.ID); len(got) != 1 || got[0].ID != pay.ID ||
		got[0].Status != repo.PaymentSucceeded {
		t.Fatalf("платежи курса до удаления: %+v, ожидали один succeeded %s", got, pay.ID)
	}
	if got := s.ListPaymentsByStudent(student.ID); len(got) != 1 || got[0].ID != pay.ID {
		t.Fatalf("платежи ученика до удаления: %+v, ожидали один %s", got, pay.ID)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM lesson_progress WHERE student_id=$1::uuid`, student.ID); n != 1 {
		t.Fatalf("строк прогресса до удаления: %d, ожидали 1", n)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM reviews WHERE course_id=$1::uuid`, course.ID); n != 1 {
		t.Fatalf("отзывов курса до удаления: %d, ожидали 1", n)
	}

	// Удаляем курс — единственное действие теста над чужим по смыслу кодом.
	if err := s.DeleteCourse(course.ID); err != nil {
		t.Fatalf("DeleteCourse(%s): %v", course.ID, err)
	}

	// Курс исчез.
	if got, ok := s.GetCourseByID(course.ID); ok {
		t.Fatalf("курс после DeleteCourse читается: %+v", got)
	}

	// Модули и уроки исчезли: методы — пусто, геттеры по id — not found.
	if got := s.ListLessonsByCourse(course.ID); len(got) != 0 {
		t.Fatalf("уроки курса после удаления: %d, ожидали 0", len(got))
	}
	for _, lid := range lessons {
		if got, ok := s.GetLesson(lid); ok {
			t.Fatalf("урок %s после удаления курса читается: %+v", lid, got)
		}
	}
	if got := s.ListModulesByCourse(course.ID); len(got) != 0 {
		t.Fatalf("модули курса после удаления: %d, ожидали 0", len(got))
	}
	for _, m := range mods {
		if got, ok := s.GetModule(m.ID); ok {
			t.Fatalf("модуль %s после удаления курса читается: %+v", m.ID, got)
		}
	}

	// Запись ученика снята каскадом.
	if got, ok := s.GetEnrollment(student.ID, course.ID); ok {
		t.Fatalf("запись ученика на курс после удаления: %+v, ожидали отсутствие", got)
	}
	for _, e := range s.ListEnrollmentsByStudent(student.ID) {
		if e.CourseID == course.ID {
			t.Fatalf("запись на удалённый курс осталась в списке ученика: %+v", e)
		}
	}

	// Платежи удалённого курса исчезли у курса и у ученика.
	if got := s.ListPaymentsByCourse(course.ID); len(got) != 0 {
		t.Fatalf("платежи удалённого курса: %d, ожидали 0", len(got))
	}
	for _, p := range s.ListPaymentsByStudent(student.ID) {
		if p.CourseID == course.ID {
			t.Fatalf("платёж удалённого курса остался у ученика: %+v", p)
		}
	}

	// Каскад не задел самих пользователей и список курсов преподавателя.
	if _, ok := s.GetUserByID(student.ID); !ok {
		t.Fatalf("ученик %s исчез вместе с курсом", student.ID)
	}
	if _, ok := s.GetUserByID(teacher.ID); !ok {
		t.Fatalf("преподаватель %s исчез вместе с курсом", teacher.ID)
	}
	for _, c := range s.ListCoursesByTeacher(teacher.ID) {
		if c.ID == course.ID {
			t.Fatalf("удалённый курс остался в списке курсов преподавателя")
		}
	}

	// Прямой контроль фактических строк по своим id: FK-каскад сработал в БД.
	if n := countRows(t, `SELECT COUNT(*) FROM modules WHERE course_id=$1::uuid`, course.ID); n != 0 {
		t.Fatalf("строк modules курса после удаления: %d, ожидали 0", n)
	}
	for _, lid := range lessons {
		if n := countRows(t, `SELECT COUNT(*) FROM lessons WHERE id=$1::uuid`, lid); n != 0 {
			t.Fatalf("строк lessons урока %s после удаления: %d, ожидали 0", lid, n)
		}
	}
	if n := countRows(t, `SELECT COUNT(*) FROM enrollments WHERE course_id=$1::uuid`, course.ID); n != 0 {
		t.Fatalf("строк enrollments курса после удаления: %d, ожидали 0", n)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM payments WHERE course_id=$1::uuid`, course.ID); n != 0 {
		t.Fatalf("строк payments курса после удаления: %d, ожидали 0", n)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM reviews WHERE course_id=$1::uuid`, course.ID); n != 0 {
		t.Fatalf("строк reviews курса после удаления: %d, ожидали 0", n)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM lesson_progress WHERE student_id=$1::uuid`, student.ID); n != 0 {
		t.Fatalf("строк lesson_progress ученика после удаления: %d, ожидали 0", n)
	}

	// Повторное удаление — ErrNotFound (как DeleteCourse у in-memory Store).
	if err := s.DeleteCourse(course.ID); err != repo.ErrNotFound {
		t.Fatalf("повторный DeleteCourse: err=%v, ожидали ErrNotFound", err)
	}
}

// TestPostgresMigrationsIdempotent — повторный прогон migrate.ApplyMigrations
// (тот же раннер, что в cmd/api) на том же пуле: файлы 001–006 должны
// применяться без ошибок и не терять данные (все действия — IF NOT EXISTS /
// ON CONFLICT / DO-блоки; см. комментарий 006_user_status.sql).
func TestPostgresMigrationsIdempotent(t *testing.T) {
	s := testStore(t) // миграции уже применены помощником: это прогон №1

	// Свои данные до повторных прогонов: миграции их не должны потерять.
	teacher := createUser(t, s, "migr-teacher", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "migr-course", 4900)

	dir, err := migrate.FindMigrationsDir(
		"../../internal/migrate/migrations",
		"internal/migrate/migrations",
	)
	if err != nil {
		t.Fatalf("каталог миграций: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for i := 1; i <= 2; i++ {
		if err := migrate.ApplyMigrations(ctx, initPool, dir); err != nil {
			t.Fatalf("повторное применение миграций (прогон %d): %v", i+1, err)
		}
	}

	// Данные живы, схема отвечает.
	if got, ok := s.GetCourseByID(course.ID); !ok || got.Slug != course.Slug {
		t.Fatalf("после повторных миграций курс не читается: ok=%v got=%+v", ok, got)
	}
	if got, ok := s.GetUserByID(teacher.ID); !ok || got.Email != teacher.Email {
		t.Fatalf("после повторных миграций пользователь не читается: ok=%v got=%+v", ok, got)
	}

	// Ключевые таблицы существуют и читаются (COUNT без ошибки).
	for _, tbl := range []string{
		"users", "teacher_invites", "courses", "modules", "lessons",
		"enrollments", "lesson_progress", "payments", "reviews",
		"sessions", "app_settings",
	} {
		countRows(t, `SELECT COUNT(*) FROM `+tbl)
	}

	// CHECK-констрейнт из 006_user_status.sql добавлен ровно один раз
	// (повторный прогон не пытается добавить его второй раз).
	if n := countRows(t, `SELECT COUNT(*) FROM pg_constraint WHERE conname='users_status_check'`); n != 1 {
		t.Fatalf("констрейнтов users_status_check: %d, ожидали 1", n)
	}

	// Строка app_settings из 001 (сид комиссии) на месте.
	var pct string
	if err := initPool.QueryRow(ctx, `SELECT value FROM app_settings WHERE key='commission_pct'`).Scan(&pct); err != nil {
		t.Fatalf("commission_pct после повторных миграций: %v", err)
	}
}
