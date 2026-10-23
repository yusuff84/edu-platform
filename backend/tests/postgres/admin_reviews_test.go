package postgres_test

// T83 на Postgres: глобальный список отзывов (фильтры courseId/rating/q/
// from-to, сортировка, пагинация, total до limit) и пересчёт рейтинга после
// удаления. Env-gated через TEST_DATABASE_URL, как остальные тесты пакета.
//
// Тест работает только со своими сущностями (runPrefix в email/slug/теле
// отзыва) и проверяет выборки с фильтром по своим курсам, чтобы не зависеть
// от данных других тестов/стенда.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

func TestPostgresAdminReviewsParity(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "t83-teacher", repo.RoleTeacher)
	courseA := createCourse(t, s, teacher.ID, "t83-a", 0)
	courseB := createCourse(t, s, teacher.ID, "t83-b", 0)

	student1 := createUser(t, s, "t83-student1", repo.RoleStudent)
	student2 := createUser(t, s, "t83-student2", repo.RoleStudent)
	student3 := createUser(t, s, "t83-student3", repo.RoleStudent)
	name1 := "Анна-" + runPrefix + "-" + shortID()
	name2 := "Борис-" + runPrefix + "-" + shortID()
	if !s.SetUserDisplayName(student1.ID, name1) || !s.SetUserDisplayName(student2.ID, name2) {
		t.Fatal("не удалось задать display_name")
	}

	marker := "T83Marker-" + shortID()
	r1, err := s.CreateReview(student1.ID, courseA.ID, 5, marker+" про Go")
	if err != nil {
		t.Fatalf("CreateReview r1: %v", err)
	}
	r2, err := s.CreateReview(student2.ID, courseA.ID, 4, "")
	if err != nil {
		t.Fatalf("CreateReview r2: %v", err)
	}
	r3, err := s.CreateReview(student3.ID, courseB.ID, 5, "Нужен Python "+runPrefix)
	if err != nil {
		t.Fatalf("CreateReview r3: %v", err)
	}
	r4, err := s.CreateReview(student1.ID, courseB.ID, 3, "Python сложный "+runPrefix)
	if err != nil {
		t.Fatalf("CreateReview r4: %v", err)
	}

	// Курс A: ровно два отзыва, новые первыми, полная форма строки.
	items, total := s.ListAdminReviews(repo.AdminReviewFilter{CourseID: courseA.ID, Limit: 100})
	if total != 2 || len(items) != 2 {
		t.Fatalf("courseA: total=%d items=%d, ждали 2/2", total, len(items))
	}
	if items[0].ID != r2.ID || items[1].ID != r1.ID {
		t.Fatalf("порядок createdAt desc нарушен: %s, %s", items[0].ID, items[1].ID)
	}
	row := items[1]
	if row.CourseID != courseA.ID || row.CourseTitle != courseA.Title || row.CourseSlug != courseA.Slug {
		t.Fatalf("данные курса в строке: %+v", row)
	}
	if row.StudentID != student1.ID || row.StudentEmail != student1.Email || row.StudentName != name1 {
		t.Fatalf("данные ученика в строке: %+v", row)
	}
	if row.Rating != 5 || row.Body != marker+" про Go" || row.TeacherReply != "" ||
		row.TeacherReplyAt != nil {
		t.Fatalf("поля отзыва в строке: %+v", row)
	}
	if items[0].Body != "" {
		t.Fatalf("body отзыва без текста=%q, ждали пустую строку", items[0].Body)
	}
	if !sameMicro(row.CreatedAt, r1.CreatedAt) {
		t.Fatalf("createdAt строки %v != созданному %v", row.CreatedAt, r1.CreatedAt)
	}

	// Нет совпадений — не-nil пустой срез и total 0.
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{CourseID: uuid.NewString(), Limit: 100})
	if items == nil || total != 0 || len(items) != 0 {
		t.Fatalf("пустая выборка: items=%v total=%d, ждали []/0", items, total)
	}

	// Фильтр rating (указатель) + courseId.
	five := 5
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{CourseID: courseA.ID, Rating: &five, Limit: 100})
	if total != 1 || len(items) != 1 || items[0].ID != r1.ID {
		t.Fatalf("rating=5 courseA: total=%d items=%+v", total, items)
	}
	two := 2
	if _, total = s.ListAdminReviews(repo.AdminReviewFilter{Rating: &two, Limit: 100}); total != 0 {
		t.Fatalf("rating=2: total=%d, ждали 0", total)
	}

	// Поиск q без учёта регистра: название курса (по префиксу прогона),
	// email/имя ученика и текст отзыва. Параметры фильтруются по своим курсам,
	// где это возможно.
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{
		Q: strings.ToUpper(runPrefix) + "-T83-A", Limit: 100,
	})
	if total != 2 || len(items) != 2 {
		t.Fatalf("q по названию курса A: total=%d items=%d, ждали 2/2", total, len(items))
	}
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseA.ID, Q: strings.ToUpper(name1), Limit: 100,
	})
	if total != 1 || len(items) != 1 || items[0].ID != r1.ID {
		t.Fatalf("q по имени ученика: total=%d items=%+v", total, items)
	}
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseB.ID, Q: strings.ToUpper(marker), Limit: 100,
	})
	if total != 0 {
		t.Fatalf("q по телу чужого курса: total=%d, ждали 0", total)
	}
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{
		Q: strings.ToLower(marker), Limit: 100,
	})
	if total != 1 || len(items) != 1 || items[0].ID != r1.ID {
		t.Fatalf("q по телу отзыва: total=%d items=%+v", total, items)
	}

	// Период from/to включительно по createdAt.
	day := r1.CreatedAt.UTC().Format("2006-01-02")
	from, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatalf("parse from: %v", err)
	}
	to := from
	if _, total = s.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseA.ID, From: from, To: to, Limit: 100,
	}); total != 2 {
		t.Fatalf("from/to текущего дня: total=%d, ждали 2", total)
	}
	next := from.AddDate(0, 0, 1)
	if _, total = s.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseA.ID, From: next, To: next, Limit: 100,
	}); total != 0 {
		t.Fatalf("from/to следующего дня: total=%d, ждали 0", total)
	}

	// Сортировка по оценке на курсе B: 3 и 5.
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseB.ID, Sort: "rating", Dir: "asc", Limit: 100,
	})
	if total != 2 || len(items) != 2 || items[0].ID != r4.ID || items[1].ID != r3.ID {
		t.Fatalf("sort=rating&dir=asc: total=%d items=%+v", total, items)
	}
	items, _ = s.ListAdminReviews(repo.AdminReviewFilter{
		CourseID: courseB.ID, Sort: "rating", Dir: "desc", Limit: 100,
	})
	if len(items) != 2 || items[0].ID != r3.ID || items[1].ID != r4.ID {
		t.Fatalf("sort=rating&dir=desc: %+v", items)
	}

	// Пагинация: total до limit.
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{CourseID: courseA.ID, Page: 1, Limit: 1})
	if total != 2 || len(items) != 1 || items[0].ID != r2.ID {
		t.Fatalf("page=1&limit=1: total=%d items=%+v", total, items)
	}
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{CourseID: courseA.ID, Page: 2, Limit: 1})
	if total != 2 || len(items) != 1 || items[0].ID != r1.ID {
		t.Fatalf("page=2&limit=1: total=%d items=%+v", total, items)
	}

	// Удаление отзыва и пересчёт рейтинга курса (T25/T83).
	s.RecalcCourseRating(courseA.ID)
	course, ok := s.GetCourseByID(courseA.ID)
	if !ok || course.AvgRating != 4.5 || course.RatingsCount != 2 {
		t.Fatalf("до удаления рейтинг курса A: ok=%v %+v", ok, course)
	}
	if err := s.DeleteReview(r1.ID); err != nil {
		t.Fatalf("DeleteReview: %v", err)
	}
	course, _ = s.GetCourseByID(courseA.ID)
	if course.AvgRating != 4 || course.RatingsCount != 1 {
		t.Fatalf("после удаления рейтинг курса A: avg=%v count=%d", course.AvgRating, course.RatingsCount)
	}
	items, total = s.ListAdminReviews(repo.AdminReviewFilter{CourseID: courseA.ID, Limit: 100})
	if total != 1 || len(items) != 1 || items[0].ID != r2.ID {
		t.Fatalf("после удаления: total=%d items=%+v", total, items)
	}
}
