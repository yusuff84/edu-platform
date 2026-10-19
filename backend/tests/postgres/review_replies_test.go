package postgres_test

// T69 — parity-тесты новых методов Postgres-хранилища: ответ преподавателя
// на отзыв (установка/замена/очистка) и разбивка оценок курса.
// Миграцию 011_review_reply.sql прогоняет сам testStore (ApplyMigrations
// падает при битом SQL/нет колонок).

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// findReview — отзыв из ListReviewsByCourse по id.
func findReview(t *testing.T, s *repo.PostgresStore, courseID, reviewID string) *repo.Review {
	t.Helper()
	for _, r := range s.ListReviewsByCourse(courseID) {
		if r.ID == reviewID {
			return r
		}
	}
	t.Fatalf("отзыв %s не найден в курсе %s", reviewID, courseID)
	return nil
}

// TestPostgresReviewReplyRoundtrip — установка, замена и очистка ответа,
// проверка teacherReply/teacherReplyAt через ListReviewsByCourse.
func TestPostgresReviewReplyRoundtrip(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "reply-teacher", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "reply", 0)
	student := createUser(t, s, "reply-s1", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)

	rev, err := s.CreateReview(student.ID, course.ID, 4, "Отзыв для ответа")
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	got := findReview(t, s, course.ID, rev.ID)
	if got.TeacherReply != "" || got.TeacherReplyAt != nil {
		t.Fatalf("новый отзыв: teacherReply=%q at=%v, ждали пусто/null", got.TeacherReply, got.TeacherReplyAt)
	}

	// Установка ответа: текст + время, видно в выборке списка.
	replied, err := s.SetReviewReply(course.ID, rev.ID, "Спасибо за отзыв!")
	if err != nil {
		t.Fatalf("SetReviewReply: %v", err)
	}
	if replied.ID != rev.ID || replied.TeacherReply != "Спасибо за отзыв!" || replied.TeacherReplyAt == nil {
		t.Fatalf("SetReviewReply вернул %+v", replied)
	}
	firstAt := *replied.TeacherReplyAt
	got = findReview(t, s, course.ID, rev.ID)
	if got.TeacherReply != "Спасибо за отзыв!" || got.TeacherReplyAt == nil {
		t.Fatalf("ответ не сохранён в БД: %+v", got)
	}
	if !sameMicro(*got.TeacherReplyAt, firstAt) {
		t.Fatalf("teacherReplyAt в БД %v != возвращённому %v", *got.TeacherReplyAt, firstAt)
	}

	// Повторный вызов заменяет текст и обновляет время.
	time.Sleep(2 * time.Millisecond)
	replied, err = s.SetReviewReply(course.ID, rev.ID, "Обновлённый ответ")
	if err != nil {
		t.Fatalf("повторный SetReviewReply: %v", err)
	}
	if replied.TeacherReply != "Обновлённый ответ" || replied.TeacherReplyAt == nil {
		t.Fatalf("повторный SetReviewReply вернул %+v", replied)
	}
	if !replied.TeacherReplyAt.After(firstAt) {
		t.Fatalf("teacherReplyAt не обновился: было %v, стало %v", firstAt, *replied.TeacherReplyAt)
	}

	// Очистка: пустая строка + NULL; повторная очистка идемпотентна.
	if err := s.ClearReviewReply(course.ID, rev.ID); err != nil {
		t.Fatalf("ClearReviewReply: %v", err)
	}
	got = findReview(t, s, course.ID, rev.ID)
	if got.TeacherReply != "" || got.TeacherReplyAt != nil {
		t.Fatalf("ответ не очищен: teacherReply=%q at=%v", got.TeacherReply, got.TeacherReplyAt)
	}
	if err := s.ClearReviewReply(course.ID, rev.ID); err != nil {
		t.Fatalf("повторный ClearReviewReply: %v", err)
	}
}

// TestPostgresReviewReplyNotFound — отзыв другого курса/несуществующий id:
// ErrNotFound, а не ошибка БД; чужая запись не меняется.
func TestPostgresReviewReplyNotFound(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "reply-nf-teacher", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "reply-nf", 0)
	other := createCourse(t, s, teacher.ID, "reply-nf-other", 0)
	student := createUser(t, s, "reply-nf-s1", repo.RoleStudent)
	enroll(t, s, student.ID, course.ID)
	rev, err := s.CreateReview(student.ID, course.ID, 5, "Отзыв")
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	// Отзыв существует, но не в этом курсе.
	if _, err := s.SetReviewReply(other.ID, rev.ID, "не туда"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("SetReviewReply(чужой курс): err=%v, ждали ErrNotFound", err)
	}
	if err := s.ClearReviewReply(other.ID, rev.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("ClearReviewReply(чужой курс): err=%v, ждали ErrNotFound", err)
	}
	// Отзыва нет вовсе (валидный UUID и мусор) — тоже ErrNotFound.
	if _, err := s.SetReviewReply(course.ID, uuid.NewString(), "нет отзыва"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("SetReviewReply(нет отзыва): err=%v, ждали ErrNotFound", err)
	}
	if _, err := s.SetReviewReply(course.ID, "not-a-uuid", "нет отзыва"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("SetReviewReply(мусорный id): err=%v, ждали ErrNotFound", err)
	}
	if err := s.ClearReviewReply(course.ID, "not-a-uuid"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("ClearReviewReply(мусорный id): err=%v, ждали ErrNotFound", err)
	}
	// Чужой курс не затронут.
	if _, err := s.SetReviewReply(other.ID, uuid.NewString(), "x"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("SetReviewReply(чужой курс/нет отзыва): err=%v", err)
	}
	// Исходный отзыв не изменился.
	got := findReview(t, s, course.ID, rev.ID)
	if got.TeacherReply != "" || got.TeacherReplyAt != nil {
		t.Fatalf("отзыв изменён неудачными вызовами: %+v", got)
	}
}

// TestPostgresReviewRatingBreakdown — разбивка по оценкам 1..5: все ключи
// присутствуют, счётчики совпадают с отзывами; пустой курс — нули.
func TestPostgresReviewRatingBreakdown(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "breakdown-teacher", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "breakdown", 0)
	empty := createCourse(t, s, teacher.ID, "breakdown-empty", 0)

	check := func(courseID string, want map[int]int) {
		t.Helper()
		got := s.ReviewRatingBreakdown(courseID)
		if len(got) != 5 {
			t.Fatalf("breakdown %s: %d ключей, ждали 5: %v", courseID, len(got), got)
		}
		for rating := 1; rating <= 5; rating++ {
			if got[rating] != want[rating] {
				t.Fatalf("breakdown[%d]=%d, ждали %d (%v)", rating, got[rating], want[rating], got)
			}
		}
	}

	check(course.ID, map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0})
	check(empty.ID, map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0})

	ratings := []int{5, 4, 4, 2}
	for i, rating := range ratings {
		student := createUser(t, s, "breakdown-s"+shortID(), repo.RoleStudent)
		enroll(t, s, student.ID, course.ID)
		if _, err := s.CreateReview(student.ID, course.ID, rating, "отзыв"); err != nil {
			t.Fatalf("CreateReview #%d: %v", i, err)
		}
	}
	check(course.ID, map[int]int{1: 0, 2: 1, 3: 0, 4: 2, 5: 1})
	// Чужой/пустой курс не задет.
	check(empty.ID, map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0})

	// Кривой id курса — нулевая разбивка, без паники/ошибки.
	check("not-a-uuid", map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0})
}
