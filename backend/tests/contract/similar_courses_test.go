package contract_test

// Контрактные тесты T79 — похожие курсы:
//   GET /api/v1/courses/{id}/similar?limit=4 — публичный.
//
// Проверяется замороженный контракт P9: подборка только published-курсов той
// же category и без самого курса; порядок «активные записи desc → avgRating
// desc → createdAt desc»; пустая категория отдаёт пустой items; limit по
// умолчанию 4, максимум 12, мусор/<=0 — дефолт; 404 для неопубликованного,
// несуществующего и некорректного id.
//
// Данные готовятся напрямую через store: нужны управляемые createdAt,
// рейтинги и число активных записей, которые через HTTP заняли бы заметно
// больше запросов; проверяется именно контракт выдачи.

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// similarSeedCourse создаёт курс напрямую в store, задаёт createdAt и
// публикует его. createdAt важен: это третий ключ сортировки подборки.
func similarSeedCourse(t *testing.T, app *handler.App, teacherID, slug, category string, createdAt time.Time) *repo.Course {
	t.Helper()
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: slug, Title: "Курс " + slug,
		Description: "Описание " + slug, Category: category,
		PriceCents: 4900, Currency: "RUB",
	})
	// CreateCourse ставит now(); для детерминированного порядка возвращаем
	// заданный момент — store хранит тот же указатель.
	c.CreatedAt = createdAt
	c.UpdatedAt = createdAt
	if !app.Store.SetCourseStatus(c.ID, repo.CoursePublished, "") {
		t.Fatalf("SetCourseStatus(%s): false", slug)
	}
	return c
}

// similarAddActiveEnrollments записывает n учеников на курс (статус active) —
// первичный ключ сортировки подборки.
func similarAddActiveEnrollments(t *testing.T, app *handler.App, courseID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		student, err := app.Store.CreateUser(
			fmt.Sprintf("similar-enroll-%s-%d@example.com", courseID, i), "hash", repo.RoleStudent)
		if err != nil {
			t.Fatalf("CreateUser для записи: %v", err)
		}
		app.Store.EnsureEnrollment(student.ID, courseID)
	}
}

// similarAddRatings создаёт по отзыву от разных учеников и пересчитывает
// рейтинг курса (как handler после CreateReview).
func similarAddRatings(t *testing.T, app *handler.App, courseID string, ratings ...int) {
	t.Helper()
	for i, rating := range ratings {
		student, err := app.Store.CreateUser(
			fmt.Sprintf("similar-rating-%s-%d@example.com", courseID, i), "hash", repo.RoleStudent)
		if err != nil {
			t.Fatalf("CreateUser для отзыва: %v", err)
		}
		if _, err := app.Store.CreateReview(student.ID, courseID, rating, ""); err != nil {
			t.Fatalf("CreateReview(%d): %v", rating, err)
		}
	}
	app.Store.RecalcCourseRating(courseID)
}

// similarGet запрашивает подборку и возвращает total и элементы, проверяя 200.
func similarGet(t *testing.T, router http.Handler, courseID, query string) (int, []map[string]any) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+courseID+"/similar"+query, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /courses/%s/similar%s: status=%d body=%s", courseID, query, rec.Code, rec.Body.String())
	}
	var out struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeBody(t, rec, &out)
	if out.Items == nil {
		t.Fatalf("items должен быть массивом (пустой — []), получено null: %s", rec.Body.String())
	}
	return out.Total, out.Items
}

// similarWantSlugs сверяет состав и порядок подборки по slug'ам.
func similarWantSlugs(t *testing.T, items []map[string]any, want ...string) {
	t.Helper()
	got := make([]string, 0, len(items))
	for _, item := range items {
		slug, _ := item["slug"].(string)
		got = append(got, slug)
	}
	if len(got) != len(want) {
		t.Fatalf("состав подборки: got=%v, want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок подборки: got=%v, want=%v (расхождение в позиции %d)", got, want, i)
		}
	}
}

// TestSimilarCoursesContract — состав, порядок, limit/дефолт и форма
// CourseItem публичного эндпоинта похожих курсов.
func TestSimilarCoursesContract(t *testing.T) {
	app, router := newTestApp()
	teacher, err := app.Store.CreateUser("similar-teacher@example.com", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Исходный published-курс: сам в подборку не попадает.
	src := similarSeedCourse(t, app, teacher.ID, "similar-src", "programming", base)
	similarAddActiveEnrollments(t, app, src.ID, 4)
	similarAddRatings(t, app, src.ID, 5, 5, 5)

	// b и a: по 2 активные записи, одинаковый рейтинг 4.5 — b новее, поэтому
	// выше (третий ключ — createdAt desc).
	a := similarSeedCourse(t, app, teacher.ID, "similar-a", "programming", base.Add(1*time.Hour))
	similarAddActiveEnrollments(t, app, a.ID, 2)
	similarAddRatings(t, app, a.ID, 5, 4)
	b := similarSeedCourse(t, app, teacher.ID, "similar-b", "programming", base.Add(2*time.Hour))
	similarAddActiveEnrollments(t, app, b.ID, 2)
	similarAddRatings(t, app, b.ID, 5, 4)
	// c: 1 запись, рейтинг 5.
	c := similarSeedCourse(t, app, teacher.ID, "similar-c", "programming", base.Add(3*time.Hour))
	similarAddActiveEnrollments(t, app, c.ID, 1)
	similarAddRatings(t, app, c.ID, 5)
	// e и d: без записей, рейтинг e (5) выше d (4.9).
	e := similarSeedCourse(t, app, teacher.ID, "similar-e", "programming", base.Add(5*time.Hour))
	similarAddRatings(t, app, e.ID, 5)
	d := similarSeedCourse(t, app, teacher.ID, "similar-d", "programming", base.Add(4*time.Hour))
	similarAddRatings(t, app, d.ID, 5, 5, 5, 5, 4) // avg 4.8
	// Курсы других категорий и не-published — не должны попадать в подборку
	// даже с большим числом записей и максимальным рейтингом.
	other := similarSeedCourse(t, app, teacher.ID, "similar-other", "design", base.Add(6*time.Hour))
	similarAddActiveEnrollments(t, app, other.ID, 10)
	similarAddRatings(t, app, other.ID, 5)
	draft := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "similar-draft", Title: "Курс similar-draft",
		Description: "черновик", Category: "programming", PriceCents: 0, Currency: "RUB",
	})
	similarAddActiveEnrollments(t, app, draft.ID, 7)
	similarAddRatings(t, app, draft.ID, 5)
	archived := similarSeedCourse(t, app, teacher.ID, "similar-archived", "programming", base.Add(7*time.Hour))
	app.Store.SetCourseStatus(archived.ID, repo.CourseArchived, "")
	similarAddActiveEnrollments(t, app, archived.ID, 8)

	t.Run("full_order_same_category_without_self", func(t *testing.T) {
		total, items := similarGet(t, router, src.ID, "?limit=12")
		if total != 5 || len(items) != 5 {
			t.Fatalf("total=%d items=%d, ждали 5 (только programming, без src)", total, len(items))
		}
		similarWantSlugs(t, items, "similar-b", "similar-a", "similar-c", "similar-e", "similar-d")
		for _, item := range items {
			if item["category"] != "programming" {
				t.Fatalf("в подборке чужая категория: %v", item["category"])
			}
			if item["id"] == src.ID {
				t.Fatalf("сам курс попал в подборку: %v", item["id"])
			}
		}
	})

	t.Run("course_item_shape", func(t *testing.T) {
		_, items := similarGet(t, router, src.ID, "?limit=1")
		if len(items) != 1 {
			t.Fatalf("limit=1: элементов %d", len(items))
		}
		item := items[0]
		for _, field := range []string{
			"id", "slug", "title", "description", "category", "coverPath",
			"priceCents", "currency", "avgRating", "ratingsCount",
		} {
			if _, ok := item[field]; !ok {
				t.Fatalf("в CourseItem нет поля %q: %v", field, item)
			}
		}
		if _, ok := item["status"]; ok {
			t.Fatalf("CourseItem не должен содержать status: %v", item)
		}
		if item["currency"] != "RUB" || item["ratingsCount"].(float64) != 2 {
			t.Fatalf("форма CourseItem: %v", item)
		}
	})

	t.Run("default_limit_4", func(t *testing.T) {
		total, items := similarGet(t, router, src.ID, "")
		if total != 4 || len(items) != 4 {
			t.Fatalf("без limit: total=%d items=%d, ждали 4", total, len(items))
		}
		similarWantSlugs(t, items, "similar-b", "similar-a", "similar-c", "similar-e")
	})

	t.Run("explicit_limit_2", func(t *testing.T) {
		total, items := similarGet(t, router, src.ID, "?limit=2")
		if total != 2 || len(items) != 2 {
			t.Fatalf("limit=2: total=%d items=%d, ждали 2", total, len(items))
		}
		similarWantSlugs(t, items, "similar-b", "similar-a")
	})

	t.Run("limit_clamped_to_12", func(t *testing.T) {
		total, items := similarGet(t, router, src.ID, "?limit=999")
		if total != 5 || len(items) != 5 {
			t.Fatalf("limit=999: total=%d items=%d, ждали все 5 (максимум 12)", total, len(items))
		}
	})

	t.Run("invalid_limit_falls_back_to_4", func(t *testing.T) {
		for _, query := range []string{"?limit=abc", "?limit=0", "?limit=-3", "?limit="} {
			total, items := similarGet(t, router, src.ID, query)
			if total != 4 || len(items) != 4 {
				t.Fatalf("%s: total=%d items=%d, ждали дефолт 4", query, total, len(items))
			}
		}
	})
}

// TestSimilarCoursesEmptyCategory — если в категории исходного курса больше
// нет published-курсов, ответ — 200 с пустым массивом items и total 0.
func TestSimilarCoursesEmptyCategory(t *testing.T) {
	app, router := newTestApp()
	teacher, err := app.Store.CreateUser("similar-empty@example.com", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}
	base := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	src := similarSeedCourse(t, app, teacher.ID, "similar-empty-src", "marketing", base)
	// Другие категории и черновик той же — не считаются похожими.
	similarSeedCourse(t, app, teacher.ID, "similar-empty-other", "ai", base.Add(time.Hour))
	draft := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "similar-empty-draft", Title: "Черновик",
		Description: "черновик", Category: "marketing", PriceCents: 0, Currency: "RUB",
	})

	total, items := similarGet(t, router, src.ID, "")
	if total != 0 || len(items) != 0 {
		t.Fatalf("пустая категория: total=%d items=%v, ждали пустой массив", total, items)
	}
	if draft.Status != repo.CourseDraft {
		t.Fatalf("тестовый черновик получил статус %q", draft.Status)
	}
}

// TestSimilarCoursesNotFound — 404 «Курс не найден» для неопубликованного,
// несуществующего и некорректного id.
func TestSimilarCoursesNotFound(t *testing.T) {
	app, router := newTestApp()
	teacher, err := app.Store.CreateUser("similar-404@example.com", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}

	draft := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "similar-404-draft", Title: "Черновик",
		Description: "черновик", Category: "programming", PriceCents: 0, Currency: "RUB",
	})
	review := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "similar-404-review", Title: "На модерации",
		Description: "review", Category: "programming", PriceCents: 0, Currency: "RUB",
	})
	app.Store.SetCourseStatus(review.ID, repo.CourseReview, "")
	archived := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "similar-404-archived", Title: "Архив",
		Description: "архив", Category: "programming", PriceCents: 0, Currency: "RUB",
	})
	app.Store.SetCourseStatus(archived.ID, repo.CourseArchived, "")

	missing := "00000000-0000-4000-8000-000000000000"
	for _, tc := range []struct {
		name string
		id   string
	}{
		{"draft", draft.ID},
		{"review", review.ID},
		{"archived", archived.ID},
		{"missing", missing},
		{"invalid_uuid", "not-a-uuid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, router, http.MethodGet, "/api/v1/courses/"+tc.id+"/similar", nil, nil)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status=%d, ждали 404 (body=%s)", rec.Code, rec.Body.String())
			}
			var out map[string]string
			decodeBody(t, rec, &out)
			if out["error"] != "Курс не найден" {
				t.Fatalf("error=%q, ждали «Курс не найден»", out["error"])
			}
		})
	}
}
