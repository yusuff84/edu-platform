package postgres_test

// T79 — parity SQL-ветки хранилища: похожие курсы (ListSimilarCourses).
//
// Проверяем то, что in-memory-контрактные тесты не ловят: один агрегирующий
// SQL-запрос с JOIN courses src, LEFT JOIN агрегатом записей и ORDER BY
// «активные записи DESC → avg_rating DESC → created_at DESC → id DESC»,
// а также нормализацию limit (по умолчанию 4, максимум 12).
//
// Изоляция на общем стенде: у всех курсов теста уникальные slug/title И
// категория (ai-<случайный суффикс>), поэтому повторный прогон на той же БД
// не конкурирует с чужими published-курсами за TOP-12. Проверки состава и
// порядка идут по id СВОИХ курсов. Чужие данные тест не меняет и не удаляет.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// publishSimilarCourse создаёт курс, задаёт createdAt через пул (от него
// зависит третий ключ сортировки) и публикует его; возвращает строку из БД.
func publishSimilarCourse(t *testing.T, s *repo.PostgresStore, teacherID, tag, category string, createdAt time.Time) *repo.Course {
	t.Helper()
	c := s.CreateCourse(&repo.Course{
		TeacherID:   teacherID,
		Slug:        uniqueSlug("similar-" + tag),
		Title:       fmt.Sprintf("Похожие курсы %s %s", runPrefix, tag),
		Description: "Курс parity-теста T79",
		Category:    category,
		PriceCents:  4900,
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

// enrollSimilarCourse записывает n учеников на курс (статус active) —
// первичный ключ сортировки подборки.
func enrollSimilarCourse(t *testing.T, s *repo.PostgresStore, courseID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		student := createUser(t, s, "similar-enroll", repo.RoleStudent)
		enroll(t, s, student.ID, courseID)
	}
}

// rateSimilarCourse добавляет по отзыву от разных учеников и пересчитывает
// рейтинг — как handler после CreateReview.
func rateSimilarCourse(t *testing.T, s *repo.PostgresStore, courseID string, ratings ...int) {
	t.Helper()
	for i, rating := range ratings {
		student := createUser(t, s, "similar-rate", repo.RoleStudent)
		if _, err := s.CreateReview(student.ID, courseID, rating, ""); err != nil {
			t.Fatalf("CreateReview(курс %s, отзыв #%d, %d): %v", courseID, i, rating, err)
		}
	}
	s.RecalcCourseRating(courseID)
}

// similarIDs переводит курсы в множество id.
func similarIDs(courses ...*repo.Course) map[string]bool {
	out := make(map[string]bool, len(courses))
	for _, c := range courses {
		out[c.ID] = true
	}
	return out
}

// filteredSimilarIDs оставляет в выдаче только id своих курсов (в порядке
// ответа) — чужие published-курсы той же категории не участвуют в проверке.
func filteredSimilarIDs(got []*repo.Course, own map[string]bool) []string {
	out := make([]string, 0, len(got))
	for _, c := range got {
		if own[c.ID] {
			out = append(out, c.ID)
		}
	}
	return out
}

// wantSimilarIDs сверяет относительный порядок своих курсов.
func wantSimilarIDs(t *testing.T, got []string, want ...*repo.Course) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("состав подборки: got=%v, ждали %d своих курсов", got, len(want))
	}
	for i := range want {
		if got[i] != want[i].ID {
			t.Fatalf("порядок подборки: позиция %d = %s, ждали %s",
				i, got[i], want[i].ID)
		}
	}
}

// TestPostgresSimilarCourses — parity ListSimilarCourses: та же категория без
// самого курса, только published; порядок записей/рейтинга/даты; limit.
func TestPostgresSimilarCourses(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "similar", repo.RoleTeacher)
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	// Уникальная категория прогона: на повторном прогоне в общей БД не
	// соревнуемся с накопленными ai-курсами за 12 слотов подборки.
	category := "ai-" + shortID()

	// Исходный курс категории прогона — сам в подборку не попадает.
	src := publishSimilarCourse(t, s, teacher.ID, "src", category, base)
	enrollSimilarCourse(t, s, src.ID, 1)
	rateSimilarCourse(t, s, src.ID, 5, 5, 5)

	// b и a: по 6 активных записей, одинаковый рейтинг 4.5 — b новее.
	a := publishSimilarCourse(t, s, teacher.ID, "a", category, base.Add(1*time.Hour))
	enrollSimilarCourse(t, s, a.ID, 6)
	rateSimilarCourse(t, s, a.ID, 5, 4)
	b := publishSimilarCourse(t, s, teacher.ID, "b", category, base.Add(2*time.Hour))
	enrollSimilarCourse(t, s, b.ID, 6)
	rateSimilarCourse(t, s, b.ID, 5, 4)
	// c: 4 записи, рейтинг 5.
	c := publishSimilarCourse(t, s, teacher.ID, "c", category, base.Add(3*time.Hour))
	enrollSimilarCourse(t, s, c.ID, 4)
	rateSimilarCourse(t, s, c.ID, 5)
	// d: 3 записи, рейтинг 5.
	d := publishSimilarCourse(t, s, teacher.ID, "d", category, base.Add(4*time.Hour))
	enrollSimilarCourse(t, s, d.ID, 3)
	rateSimilarCourse(t, s, d.ID, 5)
	// e и f: без записей, e (5.0) выше f (4.8) по рейтингу.
	e := publishSimilarCourse(t, s, teacher.ID, "e", category, base.Add(5*time.Hour))
	rateSimilarCourse(t, s, e.ID, 5)
	f := publishSimilarCourse(t, s, teacher.ID, "f", category, base.Add(6*time.Hour))
	rateSimilarCourse(t, s, f.ID, 5, 5, 5, 5, 4)

	// Черновик той же категории и published-курс другой категории — не в подборке.
	draft := s.CreateCourse(&repo.Course{
		TeacherID: teacher.ID, Slug: uniqueSlug("similar-draft"),
		Title:       fmt.Sprintf("Похожие курсы %s draft", runPrefix),
		Description: "черновик T79", Category: category, PriceCents: 0, Currency: "RUB",
	})
	enrollSimilarCourse(t, s, draft.ID, 5)
	rateSimilarCourse(t, s, draft.ID, 5)
	other := publishSimilarCourse(t, s, teacher.ID, "other", "programming", base.Add(7*time.Hour))
	enrollSimilarCourse(t, s, other.ID, 5)
	rateSimilarCourse(t, s, other.ID, 5)

	own := similarIDs(a, b, c, d, e, f)

	t.Run("order_and_filters", func(t *testing.T) {
		got := s.ListSimilarCourses(src.ID, 12)
		if len(got) > 12 {
			t.Fatalf("limit=12 вернул %d курсов", len(got))
		}
		// Все элементы — published той же категории; src/draft/other исключены.
		for _, c := range got {
			if c.Status != repo.CoursePublished {
				t.Fatalf("в подборке не published курс: %s (%s)", c.Slug, c.Status)
			}
			if c.Category != category {
				t.Fatalf("в подборке чужая категория: %s (%s)", c.Slug, c.Category)
			}
			if c.ID == src.ID || c.ID == draft.ID || c.ID == other.ID {
				t.Fatalf("в подборке лишний курс: %s", c.Slug)
			}
		}
		// Относительный порядок своих курсов: записи → рейтинг → дата.
		wantSimilarIDs(t, filteredSimilarIDs(got, own), b, a, c, d, e, f)
	})

	t.Run("default_limit_is_4", func(t *testing.T) {
		// limit <= 0 приводит store к контрактному дефолту 4.
		got := s.ListSimilarCourses(src.ID, 0)
		if len(got) != 4 {
			t.Fatalf("limit=0: %d курсов, ждали 4", len(got))
		}
		if got[0].ID != b.ID || got[1].ID != a.ID || got[2].ID != c.ID || got[3].ID != d.ID {
			t.Fatalf("limit=0: порядок %s/%s/%s/%s, ждали b/a/c/d",
				got[0].Slug, got[1].Slug, got[2].Slug, got[3].Slug)
		}
	})

	t.Run("explicit_limit_2", func(t *testing.T) {
		got := s.ListSimilarCourses(src.ID, 2)
		if len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
			t.Fatalf("limit=2: %d курсов, ждали b/a", len(got))
		}
	})

	t.Run("limit_clamped_to_12", func(t *testing.T) {
		// 6 своих + не более 12 суммарно: верхняя граница 12 соблюдена,
		// все свои курсы присутствуют (у них больше записей, чем у чужих).
		got := s.ListSimilarCourses(src.ID, 999)
		if len(got) > 12 {
			t.Fatalf("limit=999: %d курсов, ждали не больше 12", len(got))
		}
		wantSimilarIDs(t, filteredSimilarIDs(got, own), b, a, c, d, e, f)
	})

	t.Run("source_must_be_published", func(t *testing.T) {
		if got := s.ListSimilarCourses(draft.ID, 12); len(got) != 0 {
			t.Fatalf("для черновика вернулось %d курсов, ждали пусто", len(got))
		}
		missing := "00000000-0000-4000-8000-000000000000"
		if got := s.ListSimilarCourses(missing, 12); len(got) != 0 {
			t.Fatalf("для несуществующего курса вернулось %d курсов, ждали пусто", len(got))
		}
		if got := s.ListSimilarCourses("not-a-uuid", 12); len(got) != 0 {
			t.Fatalf("для некорректного id вернулось %d курсов, ждали пусто", len(got))
		}
	})
}
