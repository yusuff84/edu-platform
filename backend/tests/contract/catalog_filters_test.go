package contract_test

// Контрактные тесты T65 — категории курсов и серверные фильтры каталога:
//   GET  /categories  — шесть слагов в фиксированном порядке, счётчики published;
//   GET  /courses     — category/price/minRating/sort (+ q/page/limit);
//   POST/PATCH /teacher/courses — необязательная category и её валидация;
//   поле category во всех видах курса (каталог, страница, teacher/admin).
//
// Данные готовятся напрямую через store: нужны управляемые createdAt,
// рейтинги и число активных записей — через HTTP это заняло бы заметно
// больше запросов, а проверяется именно контракт выдачи.

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// catalogSeedCourse создаёт курс напрямую в store и публикует его. createdAt
// задаётся явно: от него зависит вторичный ключ всех сортировок.
func catalogSeedCourse(t *testing.T, app *handler.App, teacherID, slug, title, category string, priceCents int, createdAt time.Time) *repo.Course {
	t.Helper()
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: slug, Title: title,
		Description: "Описание " + title, Category: category,
		PriceCents: priceCents, Currency: "RUB",
	})
	// CreateCourse присваивает now(); для детерминированного порядка
	// возвращаем заданный момент (store хранит тот же указатель).
	c.CreatedAt = createdAt
	c.UpdatedAt = createdAt
	app.Store.SetCourseStatus(c.ID, repo.CoursePublished, "")
	return c
}

// catalogAddRatings создаёт по отзыву от разных учеников и пересчитывает
// рейтинг курса (как handler после CreateReview).
func catalogAddRatings(t *testing.T, app *handler.App, courseID string, ratings ...int) {
	t.Helper()
	for i, rating := range ratings {
		student, err := app.Store.CreateUser(
			fmt.Sprintf("rating-%s-%d@example.com", courseID, i), "hash", repo.RoleStudent)
		if err != nil {
			t.Fatalf("CreateUser для отзыва: %v", err)
		}
		if _, err := app.Store.CreateReview(student.ID, courseID, rating, ""); err != nil {
			t.Fatalf("CreateReview(%d): %v", rating, err)
		}
	}
	app.Store.RecalcCourseRating(courseID)
}

// catalogAddActiveEnrollments записывает n учеников на курс (статус active) —
// это число определяет сортировку popular.
func catalogAddActiveEnrollments(t *testing.T, app *handler.App, courseID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		student, err := app.Store.CreateUser(
			fmt.Sprintf("enroll-%s-%d@example.com", courseID, i), "hash", repo.RoleStudent)
		if err != nil {
			t.Fatalf("CreateUser для записи: %v", err)
		}
		app.Store.EnsureEnrollment(student.ID, courseID)
	}
}

// catalogSlugs возвращает slug'и элементов в порядке ответа.
func catalogSlugs(t *testing.T, items []map[string]any) []string {
	t.Helper()
	out := make([]string, 0, len(items))
	for _, item := range items {
		slug, _ := item["slug"].(string)
		out = append(out, slug)
	}
	return out
}

func wantCatalogSlugs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("порядок/состав каталога: got=%v, want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок каталога: got=%v, want=%v (расхождение в позиции %d)", got, want, i)
		}
	}
}

// catalogGet запрашивает каталог и возвращает total и элементы.
func catalogGet(t *testing.T, router http.Handler, query string) (int, []map[string]any) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses"+query, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /courses%s: status=%d body=%s", query, rec.Code, rec.Body.String())
	}
	var out struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Page  int              `json:"page"`
		Limit int              `json:"limit"`
	}
	decodeBody(t, rec, &out)
	return out.Total, out.Items
}

// catalogWantError проверяет 400 и точный текст ошибки контракта T65.
func catalogWantError(t *testing.T, router http.Handler, query, wantErr string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/courses"+query, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /courses%s: status=%d, ждали 400 (body=%s)", query, rec.Code, rec.Body.String())
	}
	var out map[string]string
	decodeBody(t, rec, &out)
	if out["error"] != wantErr {
		t.Fatalf("GET /courses%s: error=%q, ждали %q", query, out["error"], wantErr)
	}
}

// catalogTestTeacher создаёт преподавателя напрямую в store.
func catalogTestTeacher(t *testing.T, app *handler.App) *repo.User {
	t.Helper()
	teacher, err := app.Store.CreateUser("catalog-teacher@example.com", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser(teacher): %v", err)
	}
	return teacher
}

// TestCategoriesList — GET /categories: фиксированный порядок, счётчики
// только published, отсутствующие категории отдаются с 0.
func TestCategoriesList(t *testing.T) {
	app, router := newTestApp()
	teacher := catalogTestTeacher(t, app)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	catalogSeedCourse(t, app, teacher.ID, "cat-prog-1", "Программирование 1", "programming", 0, base)
	catalogSeedCourse(t, app, teacher.ID, "cat-prog-2", "Программирование 2", "programming", 1000, base.Add(time.Hour))
	catalogSeedCourse(t, app, teacher.ID, "cat-other-1", "Прочее", "other", 0, base.Add(2*time.Hour))
	// Черновик не попадает ни в каталог, ни в счётчики категорий.
	draft := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "cat-data-draft", Title: "Анализ данных (черновик)",
		Description: "черновик", Category: "data", PriceCents: 0, Currency: "RUB",
	})

	rec := doRequest(t, router, http.MethodGet, "/api/v1/categories", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /categories: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &out)

	wantOrder := []string{"programming", "design", "data", "marketing", "ai", "other"}
	if len(out.Items) != len(wantOrder) {
		t.Fatalf("категорий %d, ждали %d: %s", len(out.Items), len(wantOrder), rec.Body.String())
	}
	wantCounts := map[string]float64{
		"programming": 2, "design": 0, "data": 0, "marketing": 0, "ai": 0, "other": 1,
	}
	for i, slug := range wantOrder {
		item := out.Items[i]
		if len(item) != 2 {
			t.Fatalf("элемент категории должен содержать ровно slug/coursesCount: %v", item)
		}
		if item["slug"] != slug {
			t.Fatalf("порядок категорий: позиция %d = %v, ждали %q", i, item["slug"], slug)
		}
		wantNum(t, item, "coursesCount", wantCounts[slug])
	}
	// Черновик data не должен был посчитать категорию.
	if draft.Status != repo.CourseDraft {
		t.Fatalf("тестовый черновик получил статус %q", draft.Status)
	}
}

// TestCatalogFiltersAndSorts — данные с разными ценами, рейтингами, датами
// публикации и числом активных записей; проверяются фильтры по отдельности,
// комбинации и все сортировки.
func TestCatalogFiltersAndSorts(t *testing.T) {
	app, router := newTestApp()
	teacher := catalogTestTeacher(t, app)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// a: free, 3 активные записи, avg 4.7
	a := catalogSeedCourse(t, app, teacher.ID, "cat-a", "Python для начинающих", "programming", 0, base.Add(1*time.Hour))
	catalogAddActiveEnrollments(t, app, a.ID, 3)
	catalogAddRatings(t, app, a.ID, 5, 5, 4)
	// b: paid 50000, 1 запись, avg 4.2
	b := catalogSeedCourse(t, app, teacher.ID, "cat-b", "Дизайн интерфейсов", "design", 50000, base.Add(2*time.Hour))
	catalogAddActiveEnrollments(t, app, b.ID, 1)
	catalogAddRatings(t, app, b.ID, 4, 4, 4, 4, 5)
	// c: free, без записей и без отзывов
	catalogSeedCourse(t, app, teacher.ID, "cat-c", "Анализ данных", "data", 0, base.Add(3*time.Hour))
	// d: paid 100000, avg 5, без записей
	d := catalogSeedCourse(t, app, teacher.ID, "cat-d", "Маркетинг", "marketing", 100000, base.Add(4*time.Hour))
	catalogAddRatings(t, app, d.ID, 5)
	// e: paid 25000, 2 записи, avg 4.5
	e := catalogSeedCourse(t, app, teacher.ID, "cat-e", "AI-ассистенты", "ai", 25000, base.Add(5*time.Hour))
	catalogAddActiveEnrollments(t, app, e.ID, 2)
	catalogAddRatings(t, app, e.ID, 4, 5)
	// f: paid 10000, avg отсутствует
	catalogSeedCourse(t, app, teacher.ID, "cat-f", "Прочее направление", "other", 10000, base.Add(6*time.Hour))
	// g: черновик с большим числом записей — не должен попадать в каталог.
	g := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: "cat-g-draft", Title: "Python черновик",
		Description: "черновик", Category: "programming", PriceCents: 0, Currency: "RUB",
	})
	catalogAddActiveEnrollments(t, app, g.ID, 5)
	catalogAddRatings(t, app, g.ID, 5)

	t.Run("default_popular", func(t *testing.T) {
		total, items := catalogGet(t, router, "")
		if total != 6 {
			t.Fatalf("total=%d, ждали 6 (черновик не считается)", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-a", "cat-e", "cat-b", "cat-f", "cat-d", "cat-c")
	})

	t.Run("sort_new", func(t *testing.T) {
		total, items := catalogGet(t, router, "?sort=new")
		if total != 6 {
			t.Fatalf("total=%d, ждали 6", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-f", "cat-e", "cat-d", "cat-c", "cat-b", "cat-a")
	})

	t.Run("sort_rating", func(t *testing.T) {
		total, items := catalogGet(t, router, "?sort=rating")
		if total != 6 {
			t.Fatalf("total=%d, ждали 6", total)
		}
		// d=5, a=4.7, e=4.5, b=4.2; курсы без отзывов (0) — по createdAt desc.
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-d", "cat-a", "cat-e", "cat-b", "cat-f", "cat-c")
	})

	t.Run("sort_price_asc", func(t *testing.T) {
		total, items := catalogGet(t, router, "?sort=price_asc")
		if total != 6 {
			t.Fatalf("total=%d, ждали 6", total)
		}
		// c и a по 0 — вторичный ключ createdAt desc: c новее.
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-c", "cat-a", "cat-f", "cat-e", "cat-b", "cat-d")
	})

	t.Run("sort_price_desc", func(t *testing.T) {
		total, items := catalogGet(t, router, "?sort=price_desc")
		if total != 6 {
			t.Fatalf("total=%d, ждали 6", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-d", "cat-b", "cat-e", "cat-f", "cat-c", "cat-a")
	})

	t.Run("filter_category", func(t *testing.T) {
		total, items := catalogGet(t, router, "?category=ai")
		if total != 1 {
			t.Fatalf("total=%d, ждали 1", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-e")
		if items[0]["category"] != "ai" {
			t.Fatalf("category=%v, ждали ai", items[0]["category"])
		}
	})

	t.Run("filter_price_free", func(t *testing.T) {
		total, items := catalogGet(t, router, "?price=free")
		if total != 2 {
			t.Fatalf("total=%d, ждали 2", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-a", "cat-c")
	})

	t.Run("filter_price_paid", func(t *testing.T) {
		total, items := catalogGet(t, router, "?price=paid")
		if total != 4 {
			t.Fatalf("total=%d, ждали 4", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-e", "cat-b", "cat-f", "cat-d")
	})

	t.Run("filter_min_rating", func(t *testing.T) {
		total, items := catalogGet(t, router, "?minRating=3.5")
		if total != 4 {
			t.Fatalf("total=%d, ждали 4 (курсы без отзывов исключены)", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-a", "cat-e", "cat-b", "cat-d")
	})

	t.Run("filter_min_rating_45", func(t *testing.T) {
		total, items := catalogGet(t, router, "?minRating=4.5")
		if total != 3 {
			t.Fatalf("total=%d, ждали 3", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-a", "cat-e", "cat-d")
	})

	t.Run("filter_q", func(t *testing.T) {
		total, items := catalogGet(t, router, "?q=python")
		if total != 1 {
			t.Fatalf("total=%d, ждали 1 (draft не ищется)", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-a")
	})

	t.Run("combined", func(t *testing.T) {
		total, items := catalogGet(t, router,
			"?category=programming&price=free&minRating=4&sort=rating&q=python")
		if total != 1 {
			t.Fatalf("total=%d, ждали 1", total)
		}
		wantCatalogSlugs(t, catalogSlugs(t, items), "cat-a")
	})

	t.Run("combined_no_match", func(t *testing.T) {
		total, items := catalogGet(t, router, "?category=design&price=free")
		if total != 0 || len(items) != 0 {
			t.Fatalf("total=%d items=%v, ждали пустую выдачу", total, items)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/courses?sort=new&page=2&limit=2", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("pagination: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var out struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
			Page  int              `json:"page"`
			Limit int              `json:"limit"`
		}
		decodeBody(t, rec, &out)
		if out.Total != 6 || out.Page != 2 || out.Limit != 2 {
			t.Fatalf("total/page/limit=%d/%d/%d, ждали 6/2/2", out.Total, out.Page, out.Limit)
		}
		wantCatalogSlugs(t, catalogSlugs(t, out.Items), "cat-d", "cat-c")
	})

	t.Run("validation", func(t *testing.T) {
		catalogWantError(t, router, "?category=cooking", "Неизвестная категория")
		catalogWantError(t, router, "?price=cheap", "Недопустимый фильтр цены")
		catalogWantError(t, router, "?minRating=3.6", "Недопустимый фильтр рейтинга")
		catalogWantError(t, router, "?minRating=4.0", "Недопустимый фильтр рейтинга")
		catalogWantError(t, router, "?sort=old", "Недопустимая сортировка")
	})
}

// TestTeacherCourseCategory — создание/правка категории через teacher API,
// значение по умолчанию other, валидация и поле category во всех видах курса
// (teacher-список, страница курса, admin-список).
func TestTeacherCourseCategory(t *testing.T) {
	app, router := newTestApp()

	teacherCookies, teacherID := registerUser(t, router, "cat-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(teacherID, repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	// Без поля — other.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Курс без категории", "description": "Описание", "priceCents": 0,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create без category: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var plain map[string]any
	decodeBody(t, rec, &plain)
	plainID, _ := plain["id"].(string)
	if plain["category"] != "other" {
		t.Fatalf("category без поля=%v, ждали other", plain["category"])
	}

	// Явная категория.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "AI-курс T65", "description": "Описание", "priceCents": 1000,
		"category": "ai", "slug": "t65-ai-course",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category=ai: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var ai map[string]any
	decodeBody(t, rec, &ai)
	aiID, _ := ai["id"].(string)
	aiSlug, _ := ai["slug"].(string)
	if ai["category"] != "ai" {
		t.Fatalf("category=%v, ждали ai", ai["category"])
	}

	// Неизвестное значение — 400 с контрактным текстом.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Курс", "description": "Описание", "category": "cooking",
	}, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create category=cooking: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var errBody map[string]string
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Неизвестная категория курса" {
		t.Fatalf("error=%q, ждали «Неизвестная категория курса»", errBody["error"])
	}

	// PATCH: без поля категория не меняется.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+aiID, map[string]any{
		"title": "AI-курс T65 (переименован)",
	}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch без category: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	decodeBody(t, rec, &patched)
	if patched["category"] != "ai" {
		t.Fatalf("после PATCH без поля category=%v, ждали ai", patched["category"])
	}

	// PATCH: смена категории.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+plainID, map[string]any{
		"category": "design",
	}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch category=design: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var changed map[string]any
	decodeBody(t, rec, &changed)
	if changed["category"] != "design" {
		t.Fatalf("после PATCH category=%v, ждали design", changed["category"])
	}

	// PATCH: неизвестное/пустое значение — 400, категория прежняя.
	for _, bad := range []string{"cooking", ""} {
		rec = doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+plainID, map[string]any{
			"category": bad,
		}, teacherCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("patch category=%q: status=%d body=%s", bad, rec.Code, rec.Body.String())
		}
		var e map[string]string
		decodeBody(t, rec, &e)
		if e["error"] != "Неизвестная категория курса" {
			t.Fatalf("patch category=%q: error=%q", bad, e["error"])
		}
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+plainID, nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("get course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var detail map[string]any
	decodeBody(t, rec, &detail)
	course, _ := detail["course"].(map[string]any)
	if course["category"] != "design" {
		t.Fatalf("после неудачных PATCH category=%v, ждали design", course["category"])
	}

	// teacher-список отдаёт категории.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &list)
	got := map[string]string{}
	for _, item := range list.Items {
		id, _ := item["id"].(string)
		category, _ := item["category"].(string)
		got[id] = category
	}
	if got[plainID] != "design" || got[aiID] != "ai" {
		t.Fatalf("teacher-список: категории %v", got)
	}

	// Публикуем AI-курс и проверяем category на странице курса и в каталоге.
	app.Store.SetCourseStatus(aiID, repo.CoursePublished, "")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+aiSlug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Course map[string]any `json:"course"`
	}
	decodeBody(t, rec, &page)
	if page.Course["category"] != "ai" {
		t.Fatalf("страница курса: category=%v, ждали ai", page.Course["category"])
	}
	_, items := catalogGet(t, router, "?q=T65")
	if len(items) != 1 || items[0]["category"] != "ai" {
		t.Fatalf("каталог: %v", items)
	}

	// admin-список отдаёт category в строке.
	adminCookies, _ := registerUser(t, router, "cat-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "cat-admin@example.com")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?q=T65", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var adminList struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &adminList)
	if len(adminList.Items) == 0 {
		t.Fatalf("admin-список пуст: %s", rec.Body.String())
	}
	found := false
	for _, row := range adminList.Items {
		if row["id"] == aiID {
			found = true
			if row["category"] != "ai" {
				t.Fatalf("admin-список: category=%v, ждали ai", row["category"])
			}
		}
	}
	if !found {
		t.Fatalf("AI-курс не найден в admin-списке: %s", rec.Body.String())
	}
}
