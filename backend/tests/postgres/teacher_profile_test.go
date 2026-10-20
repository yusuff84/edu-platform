package postgres_test

// T81 — parity-тесты Postgres-ветки GetTeacherProfile (публичный профиль
// преподавателя). Проверяем то, что не поймать in-memory-тестами: реальные
// агрегирующие SQL (active-записи, средневзвешенный рейтинг), фильтр
// published и порядок «активные записи desc, createdAt desc», 404-условия.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// teacherProfilePGCourse создаёт курс с заданным createdAt и статусом.
// created_at правится через пул: от него зависит вторичный ключ порядка.
func teacherProfilePGCourse(t *testing.T, s *repo.PostgresStore, teacherID, tag string, status repo.CourseStatus, createdAt time.Time) *repo.Course {
	t.Helper()
	c := createCourse(t, s, teacherID, "tp-"+tag, 100000)
	if _, err := s.Pool().Exec(context.Background(),
		`UPDATE courses SET created_at=$2, updated_at=$2 WHERE id=$1::uuid`, c.ID, createdAt); err != nil {
		t.Fatalf("не удалось задать created_at курса %s: %v", tag, err)
	}
	if status != repo.CourseDraft && !s.SetCourseStatus(c.ID, status, "") {
		t.Fatalf("SetCourseStatus(%s, %s): false", tag, status)
	}
	got, ok := s.GetCourseByID(c.ID)
	if !ok {
		t.Fatalf("курс %s не найден после подготовки", tag)
	}
	return got
}

// teacherProfilePGRate добавляет отзывы разных учеников и пересчитывает
// рейтинг курса — как handler после CreateReview.
func teacherProfilePGRate(t *testing.T, s *repo.PostgresStore, courseID string, ratings ...int) {
	t.Helper()
	for i, rating := range ratings {
		student := createUser(t, s, "tp-rate", repo.RoleStudent)
		if _, err := s.CreateReview(student.ID, courseID, rating, ""); err != nil {
			t.Fatalf("CreateReview(курс %s, отзыв #%d, %d): %v", courseID, i, rating, err)
		}
	}
	s.RecalcCourseRating(courseID)
}

// TestPostgresTeacherProfile — счётчики только по published-курсам, состав и
// порядок курсов, пустой профиль и 404-условия (нет пользователя, student).
func TestPostgresTeacherProfile(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "tp-teacher", repo.RoleTeacher)
	if !s.SetUserDisplayName(teacher.ID, "Пётр Сидоров") {
		t.Fatal("SetUserDisplayName: false")
	}

	base := time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC)
	// A: 3 активные записи, две оценки (4.5).
	a := teacherProfilePGCourse(t, s, teacher.ID, "a", repo.CoursePublished, base)
	for i := 0; i < 3; i++ {
		enroll(t, s, createUser(t, s, "tp-enroll-a", repo.RoleStudent).ID, a.ID)
	}
	teacherProfilePGRate(t, s, a.ID, 5, 4)
	// B: 5 активных записей, одна оценка 3.0 — первый по популярности.
	b := teacherProfilePGCourse(t, s, teacher.ID, "b", repo.CoursePublished, base.Add(2*time.Hour))
	for i := 0; i < 5; i++ {
		enroll(t, s, createUser(t, s, "tp-enroll-b", repo.RoleStudent).ID, b.ID)
	}
	teacherProfilePGRate(t, s, b.ID, 3)
	// C и F: без записей; C новее — порядок по createdAt desc.
	c := teacherProfilePGCourse(t, s, teacher.ID, "c", repo.CoursePublished, base.Add(4*time.Hour))
	f := teacherProfilePGCourse(t, s, teacher.ID, "f", repo.CoursePublished, base.Add(3*time.Hour))
	// draft и archived не входят ни в курсы, ни в агрегаты.
	d := teacherProfilePGCourse(t, s, teacher.ID, "d", repo.CourseDraft, base.Add(5*time.Hour))
	for i := 0; i < 10; i++ {
		enroll(t, s, createUser(t, s, "tp-enroll-d", repo.RoleStudent).ID, d.ID)
	}
	teacherProfilePGRate(t, s, d.ID, 5)
	e := teacherProfilePGCourse(t, s, teacher.ID, "e", repo.CourseArchived, base.Add(6*time.Hour))
	for i := 0; i < 5; i++ {
		enroll(t, s, createUser(t, s, "tp-enroll-e", repo.RoleStudent).ID, e.ID)
	}
	teacherProfilePGRate(t, s, e.ID, 5)

	data, ok := s.GetTeacherProfile(teacher.ID)
	if !ok {
		t.Fatal("GetTeacherProfile: false для существующего teacher")
	}
	if data.Teacher.ID != teacher.ID || data.Teacher.DisplayName != "Пётр Сидоров" {
		t.Fatalf("карточка: %+v", data.Teacher)
	}
	if data.Teacher.CoursesCount != 4 {
		t.Fatalf("coursesCount=%d, ждали 4 (A,B,C,F published)", data.Teacher.CoursesCount)
	}
	if data.Teacher.StudentsCount != 8 {
		t.Fatalf("studentsCount=%d, ждали 8 (3+5 активных на published)", data.Teacher.StudentsCount)
	}
	if data.Teacher.RatingsCount != 3 {
		t.Fatalf("ratingsCount=%d, ждали 3 (2 у A + 1 у B)", data.Teacher.RatingsCount)
	}
	// (4.5*2 + 3.0*1)/3 = 4.0
	if data.Teacher.AvgRating != 4 {
		t.Fatalf("avgRating=%v, ждали 4.0", data.Teacher.AvgRating)
	}

	want := []string{b.ID, a.ID, c.ID, f.ID}
	if len(data.Courses) != len(want) {
		slugs := make([]string, 0, len(data.Courses))
		for _, course := range data.Courses {
			slugs = append(slugs, course.Slug)
		}
		t.Fatalf("состав курсов: %d (%v), ждали %d", len(data.Courses), slugs, len(want))
	}
	for i := range want {
		if data.Courses[i].ID != want[i] {
			t.Fatalf("порядок курсов: позиция %d = %s, ждали %s", i, data.Courses[i].Slug, want[i])
		}
	}
	for _, course := range data.Courses {
		if course.Status != repo.CoursePublished {
			t.Fatalf("в профиль попал не published курс %s (%s)", course.Slug, course.Status)
		}
	}
}

// TestPostgresTeacherProfileEmptyAndNotFound — учитель без курсов: нули и
// непустой слайс; student/несуществующий/мусорный id: false.
func TestPostgresTeacherProfileEmptyAndNotFound(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "tp-empty", repo.RoleTeacher)

	data, ok := s.GetTeacherProfile(teacher.ID)
	if !ok {
		t.Fatal("GetTeacherProfile(пустой teacher): false, ждали true")
	}
	if data.Teacher.DisplayName != "" {
		t.Fatalf("displayName=%q, ждали пустую строку", data.Teacher.DisplayName)
	}
	if data.Teacher.CoursesCount != 0 || data.Teacher.StudentsCount != 0 ||
		data.Teacher.RatingsCount != 0 || data.Teacher.AvgRating != 0 {
		t.Fatalf("агрегаты пустого профиля: %+v", data.Teacher)
	}
	if data.Courses == nil {
		t.Fatal("courses=nil, ждали пустой слайс")
	}
	if len(data.Courses) != 0 {
		t.Fatalf("courses=%d, ждали 0", len(data.Courses))
	}

	student := createUser(t, s, "tp-student", repo.RoleStudent)
	if _, ok := s.GetTeacherProfile(student.ID); ok {
		t.Fatal("GetTeacherProfile(student): ok=true, ждали false")
	}
	if _, ok := s.GetTeacherProfile(uuid.NewString()); ok {
		t.Fatal("GetTeacherProfile(несуществующий UUID): ok=true, ждали false")
	}
	if _, ok := s.GetTeacherProfile("not-a-uuid"); ok {
		t.Fatal("GetTeacherProfile(не UUID): ok=true, ждали false")
	}
}
