package contract_test

// T81 — публичный профиль преподавателя (волна P9).
// Контракт — Task/00-api-contract.md, раздел «T81 — Публичный профиль
// преподавателя» (заморожен):
//   GET /api/v1/teachers/{id} — публичный, без авторизации;
//   200 {"teacher":{id,displayName,coursesCount,studentsCount,avgRating,
//        ratingsCount},"courses":[CourseItem]};
//   404 «Преподаватель не найден» — пользователя нет или роль не teacher/admin;
//   курсы — только published, порядок: активные записи desc, затем createdAt desc.

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// teacherProfileCourse создаёт курс напрямую в store с заданным createdAt и
// статусом: через HTTP не получить управляемые даты и статусы для проверки
// состава/порядка ответа.
func teacherProfileCourse(t *testing.T, app *handler.App, teacherID, slug, title string, status repo.CourseStatus, createdAt time.Time) *repo.Course {
	t.Helper()
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: slug, Title: title,
		Description: "Описание " + title, PriceCents: 100000, Currency: "RUB",
	})
	if c.ID == "" {
		t.Fatalf("CreateCourse(%s) не вернул id", slug)
	}
	// CreateCourse присваивает now(); store хранит тот же указатель.
	c.CreatedAt = createdAt
	c.UpdatedAt = createdAt
	if status != repo.CourseDraft {
		if !app.Store.SetCourseStatus(c.ID, status, "") {
			t.Fatalf("SetCourseStatus(%s, %s): false", slug, status)
		}
	}
	return c
}

// teacherProfileEnroll записывает n активных учеников на курс.
func teacherProfileEnroll(t *testing.T, app *handler.App, courseID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		student, err := app.Store.CreateUser(
			fmt.Sprintf("tp-enroll-%s-%d@example.com", courseID, i), "hash", repo.RoleStudent)
		if err != nil {
			t.Fatalf("CreateUser для записи #%d: %v", i, err)
		}
		app.Store.EnsureEnrollment(student.ID, courseID)
	}
}

// teacherProfileRate добавляет по отзыву от разных учеников и пересчитывает
// рейтинг курса — как handler после CreateReview.
func teacherProfileRate(t *testing.T, app *handler.App, courseID string, ratings ...int) {
	t.Helper()
	for i, rating := range ratings {
		student, err := app.Store.CreateUser(
			fmt.Sprintf("tp-rate-%s-%d@example.com", courseID, i), "hash", repo.RoleStudent)
		if err != nil {
			t.Fatalf("CreateUser для отзыва #%d: %v", i, err)
		}
		if _, err := app.Store.CreateReview(student.ID, courseID, rating, ""); err != nil {
			t.Fatalf("CreateReview(%d): %v", rating, err)
		}
	}
	app.Store.RecalcCourseRating(courseID)
}

// teacherProfileSlugs — slug'и курсов в порядке ответа.
func teacherProfileSlugs(t *testing.T, courses []any) []string {
	t.Helper()
	out := make([]string, 0, len(courses))
	for _, raw := range courses {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("элемент courses не объект: %v", raw)
		}
		slug, _ := item["slug"].(string)
		out = append(out, slug)
	}
	return out
}

// TestTeacherProfilePublicCountersAndOrder — профиль доступен без входа;
// счётчики считаются только по published-курсам; курсы отсортированы по
// активным записям desc, затем createdAt desc.
func TestTeacherProfilePublicCountersAndOrder(t *testing.T) {
	app, router := newTestApp()

	teacher, err := app.Store.CreateUser("profile-teacher@example.com", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}
	if !app.Store.SetUserDisplayName(teacher.ID, "Иван Петров") {
		t.Fatal("SetUserDisplayName: false")
	}

	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	// A: 3 активные записи, две оценки (4.5).
	a := teacherProfileCourse(t, app, teacher.ID, "tp-a", "Курс A", repo.CoursePublished, base)
	teacherProfileEnroll(t, app, a.ID, 3)
	teacherProfileRate(t, app, a.ID, 5, 4)
	// B: 5 активных записей, одна оценка 3.0 — первый по популярности.
	b := teacherProfileCourse(t, app, teacher.ID, "tp-b", "Курс B", repo.CoursePublished, base.Add(2*time.Hour))
	teacherProfileEnroll(t, app, b.ID, 5)
	teacherProfileRate(t, app, b.ID, 3)
	// C и F: без записей; C новее — порядок между ними по createdAt desc.
	c := teacherProfileCourse(t, app, teacher.ID, "tp-c", "Курс C", repo.CoursePublished, base.Add(4*time.Hour))
	f := teacherProfileCourse(t, app, teacher.ID, "tp-f", "Курс F", repo.CoursePublished, base.Add(3*time.Hour))
	// draft и archived не входят ни в курсы, ни в агрегаты.
	d := teacherProfileCourse(t, app, teacher.ID, "tp-d", "Черновик", repo.CourseDraft, base.Add(5*time.Hour))
	teacherProfileEnroll(t, app, d.ID, 10)
	teacherProfileRate(t, app, d.ID, 5)
	e := teacherProfileCourse(t, app, teacher.ID, "tp-e", "Архив", repo.CourseArchived, base.Add(6*time.Hour))
	teacherProfileEnroll(t, app, e.ID, 5)
	teacherProfileRate(t, app, e.ID, 5)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/teachers/"+teacher.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /teachers/{id} без входа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeBody(t, rec, &body)

	card, ok := body["teacher"].(map[string]any)
	if !ok {
		t.Fatalf("teacher не объект: %s", rec.Body.String())
	}
	if card["id"] != teacher.ID {
		t.Fatalf("teacher.id=%v, ждали %s", card["id"], teacher.ID)
	}
	if card["displayName"] != "Иван Петров" {
		t.Fatalf("teacher.displayName=%v", card["displayName"])
	}
	want := map[string]float64{
		"coursesCount":  4, // A, B, C, F (published)
		"studentsCount": 8, // 3 + 5 активных на published
		"ratingsCount":  3, // 2 у A + 1 у B
		"avgRating":     4, // (4.5*2 + 3.0*1) / 3
	}
	for key, expected := range want {
		got, _ := card[key].(float64)
		if got != expected {
			t.Fatalf("teacher.%s=%v, ждали %v (%s)", key, card[key], expected, rec.Body.String())
		}
	}

	courses, ok := body["courses"].([]any)
	if !ok || courses == nil {
		t.Fatalf("courses не массив: %s", rec.Body.String())
	}
	gotSlugs := teacherProfileSlugs(t, courses)
	wantSlugs := []string{b.Slug, a.Slug, c.Slug, f.Slug}
	if len(gotSlugs) != len(wantSlugs) {
		t.Fatalf("состав курсов: got=%v, want=%v", gotSlugs, wantSlugs)
	}
	for i := range wantSlugs {
		if gotSlugs[i] != wantSlugs[i] {
			t.Fatalf("порядок курсов: got=%v, want=%v (позиция %d)", gotSlugs, wantSlugs, i)
		}
	}

	// Элемент — публичный CourseItem: без teacherId/status/createdAt.
	first := courses[0].(map[string]any)
	for _, key := range []string{"id", "slug", "title", "description", "category", "coverPath", "priceCents", "currency", "avgRating", "ratingsCount"} {
		if _, has := first[key]; !has {
			t.Fatalf("в CourseItem нет поля %q: %s", key, rec.Body.String())
		}
	}
	if _, has := first["teacherId"]; has {
		t.Fatalf("CourseItem не должен содержать teacherId: %s", rec.Body.String())
	}
}

// TestTeacherProfileNotFound — несуществующий id, student-аккаунт и мусорный
// id дают 404 с текстом контракта; admin допускается (200).
func TestTeacherProfileNotFound(t *testing.T) {
	app, router := newTestApp()

	student, err := app.Store.CreateUser("profile-student@example.com", "hash", repo.RoleStudent)
	if err != nil {
		t.Fatalf("CreateUser(student): %v", err)
	}

	cases := []struct{ name, id string }{
		{"несуществующий UUID", "00000000-0000-0000-0000-000000000000"},
		{"student-аккаунт", student.ID},
		{"не UUID", "not-a-uuid"},
	}
	for _, tc := range cases {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/teachers/"+tc.id, nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status=%d, ждали 404 (body=%s)", tc.name, rec.Code, rec.Body.String())
		}
		var errBody map[string]string
		decodeBody(t, rec, &errBody)
		if errBody["error"] != "Преподаватель не найден" {
			t.Fatalf("%s: текст 404=%q, ждали «Преподаватель не найден»", tc.name, errBody["error"])
		}
	}

	// admin — допустимая роль профиля (контракт: teacher/admin).
	admin, err := app.Store.CreateUser("profile-admin@example.com", "hash", repo.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser(admin): %v", err)
	}
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teachers/"+admin.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin-профиль: status=%d, ждали 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestTeacherProfileEmptyDisplayName — пустой displayName отдаётся пустой
// строкой (поле не пропадает), а courses — пустым массивом, не null.
func TestTeacherProfileEmptyDisplayName(t *testing.T) {
	app, router := newTestApp()

	teacher, err := app.Store.CreateUser("profile-noname@example.com", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}

	rec := doRequest(t, router, http.MethodGet, "/api/v1/teachers/"+teacher.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeBody(t, rec, &body)

	card, ok := body["teacher"].(map[string]any)
	if !ok {
		t.Fatalf("teacher не объект: %s", rec.Body.String())
	}
	display, has := card["displayName"]
	if !has || display != "" {
		t.Fatalf("displayName=%v (поле есть=%v), ждали пустую строку", display, has)
	}
	for _, key := range []string{"coursesCount", "studentsCount", "ratingsCount", "avgRating"} {
		if got, _ := card[key].(float64); got != 0 {
			t.Fatalf("teacher.%s=%v, ждали 0", key, card[key])
		}
	}
	courses, ok := body["courses"].([]any)
	if !ok || courses == nil {
		t.Fatalf("courses должен быть пустым массивом, не null: %s", rec.Body.String())
	}
	if len(courses) != 0 {
		t.Fatalf("courses: %d элементов, ждали 0: %s", len(courses), rec.Body.String())
	}
}
