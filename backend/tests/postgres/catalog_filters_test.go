package postgres_test

// T65 — parity SQL-ветки хранилища: категории курсов и серверные фильтры
// каталога.
//
// Проверяем то, что in-memory-тесты не ловят: миграция 009 (NOT NULL DEFAULT
// 'other'), реальные запросы ListCatalogCourses с WHERE/ORDER BY, счётчики
// CountPublishedCoursesByCategory, а также сохранение category при
// UpdateCourse/CloneCourse/AdminCourseDetail.
//
// Данные изолированы: у всех курсов теста в title/description есть уникальный
// token (префикс прогона + shortID), поэтому Q-фильтр возвращает только курсы
// этого теста. Счётчики категорий проверяются дельтами до/после — на общем
// стенде в БД могут быть чужие published-курсы.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// publishCatalogCourse создаёт курс, публикует его и возвращает прочитанную
// из БД строку. createdAt задаётся через пул: от него зависит вторичный ключ
// сортировок; SetCourseStatus в Postgres обновляет только updated_at.
func publishCatalogCourse(t *testing.T, s *repo.PostgresStore, teacherID, token, tag, category string, priceCents int, createdAt time.Time) *repo.Course {
	t.Helper()
	c := s.CreateCourse(&repo.Course{
		TeacherID:   teacherID,
		Slug:        uniqueSlug("catalog-" + tag),
		Title:       token + " " + tag,
		Description: "Описание каталога " + token,
		Category:    category,
		PriceCents:  priceCents,
		Currency:    "RUB",
	})
	if c.ID == "" {
		t.Fatalf("CreateCourse(%s) не вернул id", tag)
	}
	if _, err := s.Pool().Exec(context.Background(),
		`UPDATE courses SET created_at=$2, updated_at=$2 WHERE id=$1::uuid`,
		c.ID, createdAt); err != nil {
		t.Fatalf("не удалось задать created_at курса %s: %v", tag, err)
	}
	if !s.SetCourseStatus(c.ID, repo.CoursePublished, "") {
		t.Fatalf("SetCourseStatus(%s): false", tag)
	}
	got, ok := s.GetCourseByID(c.ID)
	if !ok {
		t.Fatalf("курс %s не найден после публикации", tag)
	}
	return got
}

// rateCatalogCourse добавляет по отзыву от разных учеников и пересчитывает
// рейтинг — как handler после CreateReview.
func rateCatalogCourse(t *testing.T, s *repo.PostgresStore, courseID string, ratings ...int) {
	t.Helper()
	for i, rating := range ratings {
		student := createUser(t, s, "catalog-rate", repo.RoleStudent)
		if _, err := s.CreateReview(student.ID, courseID, rating, ""); err != nil {
			t.Fatalf("CreateReview(курс %s, отзыв #%d, %d): %v", courseID, i, rating, err)
		}
	}
	s.RecalcCourseRating(courseID)
}

// enrollCatalogCourse записывает n учеников на курс (статус active) — это
// число определяет сортировку popular.
func enrollCatalogCourse(t *testing.T, s *repo.PostgresStore, courseID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		student := createUser(t, s, "catalog-enroll", repo.RoleStudent)
		enroll(t, s, student.ID, courseID)
	}
}

// wantCatalogCourses сверяет состав и порядок выдачи с ожидаемым.
func wantCatalogCourses(t *testing.T, got []*repo.Course, want ...*repo.Course) {
	t.Helper()
	if len(got) != len(want) {
		gotSlugs := make([]string, 0, len(got))
		for _, c := range got {
			gotSlugs = append(gotSlugs, c.Slug)
		}
		t.Fatalf("состав каталога: got=%v, ждали %d курсов", gotSlugs, len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Fatalf("порядок каталога: позиция %d = %s, ждали %s", i, got[i].Slug, want[i].Slug)
		}
	}
}

// TestPostgresCourseCategory — category переживает create/get/update/clone и
// попадает в teacher-список и админскую деталь; пустая категория становится
// other, PATCH без поля её не меняет.
func TestPostgresCourseCategory(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "category", repo.RoleTeacher)

	// Пустая категория → other (как DEFAULT миграции 009).
	plain := s.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: uniqueSlug("category-plain"),
		Title: "Курс без категории", Description: "T65: category по умолчанию",
		PriceCents: 0, Currency: "RUB",
	})
	if plain.Category != "other" {
		t.Fatalf("CreateCourse без category: %q, ждали other", plain.Category)
	}
	got, ok := s.GetCourseByID(plain.ID)
	if !ok || got.Category != "other" {
		t.Fatalf("category не сохранена в БД: ok=%v category=%q", ok, got.Category)
	}

	// Явная категория.
	ai := s.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: uniqueSlug("category-ai"),
		Title: "AI-курс T65", Description: "T65: явная категория",
		Category: "ai", PriceCents: 1000, Currency: "RUB",
	})
	if ai.Category != "ai" {
		t.Fatalf("CreateCourse(category=ai): %q", ai.Category)
	}

	// Teacher-список отдаёт категорию каждой строки.
	byID := make(map[string]string)
	for _, c := range s.ListCoursesByTeacher(teacher.ID) {
		byID[c.ID] = c.Category
	}
	if byID[plain.ID] != "other" || byID[ai.ID] != "ai" {
		t.Fatalf("ListCoursesByTeacher: категории %v", byID)
	}

	// UpdateCourse меняет category…
	design := "design"
	if !s.UpdateCourse(plain.ID, repo.CourseUpdate{Category: &design}) {
		t.Fatal("UpdateCourse(category=design): false")
	}
	got, ok = s.GetCourseByID(plain.ID)
	if !ok || got.Category != "design" {
		t.Fatalf("category после UpdateCourse: ok=%v category=%q", ok, got.Category)
	}
	// …а PATCH без поля category её не затирает.
	title := "Курс без категории (обновлён)"
	if !s.UpdateCourse(plain.ID, repo.CourseUpdate{Title: &title}) {
		t.Fatal("UpdateCourse(title): false")
	}
	got, ok = s.GetCourseByID(plain.ID)
	if !ok || got.Category != "design" || got.Title != title {
		t.Fatalf("UpdateCourse(title) затронул category: ok=%v got=%+v", ok, got)
	}

	// Админская деталь и копия курса сохраняют категорию.
	detail, ok := s.AdminCourseDetail(ai.ID)
	if !ok || detail.Course.Category != "ai" {
		t.Fatalf("AdminCourseDetail: ok=%v category=%q", ok, detail.Course.Category)
	}
	clone, err := s.CloneCourse(ai.ID)
	if err != nil {
		t.Fatalf("CloneCourse: %v", err)
	}
	if clone.Category != "ai" {
		t.Fatalf("CloneCourse: category=%q, ждали ai", clone.Category)
	}
}

// TestPostgresCategoryColumnDefault — миграция 009: INSERT без колонки
// category получает 'other' на уровне схемы (а не только через store).
func TestPostgresCategoryColumnDefault(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "category-default", repo.RoleTeacher)
	slug := uniqueSlug("category-default")

	var id string
	err := s.Pool().QueryRow(context.Background(),
		`INSERT INTO courses (teacher_id, slug, title, description, price_cents, currency, status)
		 VALUES ($1::uuid,$2,$3,$4,0,'RUB','draft') RETURNING id::text`,
		teacher.ID, slug, "Курс с DEFAULT category", "T65: миграция 009").Scan(&id)
	if err != nil {
		t.Fatalf("INSERT без category: %v", err)
	}
	got, ok := s.GetCourseByID(id)
	if !ok || got.Category != "other" {
		t.Fatalf("DEFAULT category: ok=%v category=%q, ждали other", ok, got.Category)
	}
}

// TestPostgresCatalogFiltersAndSorts — фильтры category/price/minRating/q,
// все сортировки, только published и счётчики категорий на SQL-ветке.
func TestPostgresCatalogFiltersAndSorts(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "catalog", repo.RoleTeacher)
	token := fmt.Sprintf("%s-catalog-%s", runPrefix, shortID())
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)

	before := s.CountPublishedCoursesByCategory()

	// a: free, 3 активные записи, avg 4.7 (5,5,4)
	a := publishCatalogCourse(t, s, teacher.ID, token, "a", "programming", 0, base.Add(1*time.Hour))
	enrollCatalogCourse(t, s, a.ID, 3)
	rateCatalogCourse(t, s, a.ID, 5, 5, 4)
	// b: paid 50000, 1 запись, avg 4.2 (4,4,4,4,5)
	b := publishCatalogCourse(t, s, teacher.ID, token, "b", "design", 50000, base.Add(2*time.Hour))
	enrollCatalogCourse(t, s, b.ID, 1)
	rateCatalogCourse(t, s, b.ID, 4, 4, 4, 4, 5)
	// c: free, без записей и отзывов
	c := publishCatalogCourse(t, s, teacher.ID, token, "c", "data", 0, base.Add(3*time.Hour))
	// d: paid 100000, avg 5, без записей
	d := publishCatalogCourse(t, s, teacher.ID, token, "d", "marketing", 100000, base.Add(4*time.Hour))
	rateCatalogCourse(t, s, d.ID, 5)
	// e: paid 25000, 2 записи, avg 4.5 (4,5)
	e := publishCatalogCourse(t, s, teacher.ID, token, "e", "ai", 25000, base.Add(5*time.Hour))
	enrollCatalogCourse(t, s, e.ID, 2)
	rateCatalogCourse(t, s, e.ID, 4, 5)
	// f: paid 10000, без отзывов
	f := publishCatalogCourse(t, s, teacher.ID, token, "f", "other", 10000, base.Add(6*time.Hour))

	// g: черновик — не попадает ни в каталог, ни в счётчики, несмотря на
	// рейтинг 5 и 5 активных записей.
	g := s.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: uniqueSlug("catalog-draft"),
		Title: token + " draft", Description: "Описание каталога " + token,
		Category: "programming", PriceCents: 0, Currency: "RUB",
	})
	enrollCatalogCourse(t, s, g.ID, 5)
	rateCatalogCourse(t, s, g.ID, 5)

	if got, _ := s.GetCourseByID(g.ID); got.Status != repo.CourseDraft {
		t.Fatalf("тестовый черновик получил статус %q", got.Status)
	}

	// categories: ровно +1 published на каждую из шести категорий; черновик
	// programming не должен увеличить её счётчик.
	after := s.CountPublishedCoursesByCategory()
	wantDelta := map[string]int{
		"programming": 1, "design": 1, "data": 1, "marketing": 1, "ai": 1, "other": 1,
	}
	for category, delta := range wantDelta {
		if got := after[category] - before[category]; got != delta {
			t.Fatalf("CountPublishedCoursesByCategory(%s): дельта %d, ждали %d (before=%d after=%d)",
				category, got, delta, before[category], after[category])
		}
	}

	list := func(filter repo.CatalogFilter) []*repo.Course {
		filter.Q = token
		return s.ListCatalogCourses(filter)
	}

	t.Run("only_published", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{}), a, e, b, f, d, c)
	})

	t.Run("default_equals_popular", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{}), list(repo.CatalogFilter{Sort: "popular"})...)
	})

	t.Run("sort_new", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{Sort: "new"}), f, e, d, c, b, a)
	})

	t.Run("sort_rating", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{Sort: "rating"}), d, a, e, b, f, c)
	})

	t.Run("sort_price_asc", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{Sort: "price_asc"}), c, a, f, e, b, d)
	})

	t.Run("sort_price_desc", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{Sort: "price_desc"}), d, b, e, f, c, a)
	})

	t.Run("filter_category", func(t *testing.T) {
		got := list(repo.CatalogFilter{Category: "ai"})
		wantCatalogCourses(t, got, e)
		if got[0].Category != "ai" {
			t.Fatalf("category=%q, ждали ai", got[0].Category)
		}
	})

	t.Run("filter_price_free", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{Price: "free"}), a, c)
	})

	t.Run("filter_price_paid", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{Price: "paid"}), e, b, f, d)
	})

	t.Run("filter_min_rating", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{MinRating: 3.5}), a, e, b, d)
	})

	t.Run("filter_min_rating_45", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{MinRating: 4.5}), a, e, d)
	})

	t.Run("filter_q", func(t *testing.T) {
		got := s.ListCatalogCourses(repo.CatalogFilter{Q: token + " a"})
		wantCatalogCourses(t, got, a)
	})

	t.Run("combined", func(t *testing.T) {
		wantCatalogCourses(t, list(repo.CatalogFilter{
			Category: "programming", Price: "free", MinRating: 4, Sort: "rating",
		}), a)
	})

	t.Run("combined_no_match", func(t *testing.T) {
		if got := list(repo.CatalogFilter{Category: "design", Price: "free"}); len(got) != 0 {
			t.Fatalf("category=design&price=free: %d курсов, ждали пустую выдачу", len(got))
		}
	})
}
