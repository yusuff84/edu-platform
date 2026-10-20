package postgres_test

// T73 — parity-тесты избранного на настоящем Postgres: идемпотентность
// добавления/удаления, порядок «новые первыми», фильтр published,
// изоляция пользователей и каскад 013_favorites.sql (удаление курса и
// пользователя). Env-gated через TEST_DATABASE_URL (см. helper_test.go).

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

// countFavorites — число строк избранного по произвольному условию:
// нужен для проверки каскада на уровне таблицы (список этого не покажет —
// удалённый курс в него и так не попадает).
func countFavorites(t *testing.T, s *repo.PostgresStore, where string, args ...any) int {
	t.Helper()
	var n int
	if err := s.Pool().QueryRow(t.Context(),
		"SELECT COUNT(*)::int FROM favorites WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatalf("COUNT favorites (%s): %v", where, err)
	}
	return n
}

// favoriteIDs — id курсов избранного в порядке ответа.
func favoriteIDs(courses []*repo.Course) []string {
	ids := make([]string, 0, len(courses))
	for _, c := range courses {
		ids = append(ids, c.ID)
	}
	return ids
}

// TestFavoritesParity: in-memory и Postgres ведут себя одинаково по контракту
// StoreAPI: повторное добавление не дублирует и не сдвигает порядок,
// список — только published и новые первыми, чужое избранное не видно.
func TestFavoritesParity(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "t73-teacher", repo.RoleTeacher)
	alice := createUser(t, s, "t73-alice", repo.RoleStudent)
	bob := createUser(t, s, "t73-bob", repo.RoleStudent)

	first := createCourse(t, s, teacher.ID, "t73-first", 1000)
	second := createCourse(t, s, teacher.ID, "t73-second", 2000)
	draft := createCourse(t, s, teacher.ID, "t73-draft", 3000)
	archived := createCourse(t, s, teacher.ID, "t73-archived", 4000)
	for _, c := range []*repo.Course{first, second, archived} {
		if !s.SetCourseStatus(c.ID, repo.CoursePublished, "") {
			t.Fatalf("SetCourseStatus(%s, published) не сработал", c.ID)
		}
	}

	if got := s.ListFavoriteCoursesByUser(alice.ID); got == nil || len(got) != 0 {
		t.Fatalf("пустое избранное должно быть не nil и длины 0: %#v", got)
	}

	// Добавление published-курсов и черновика: черновик в списке не появится.
	for _, id := range []string{first.ID, second.ID, draft.ID} {
		if err := s.AddFavorite(alice.ID, id); err != nil {
			t.Fatalf("AddFavorite(%s): %v", id, err)
		}
	}
	// Повторное добавление first не должно ни дублировать, ни менять порядок.
	if err := s.AddFavorite(alice.ID, first.ID); err != nil {
		t.Fatalf("повторный AddFavorite(first): %v", err)
	}

	got := s.ListFavoriteCoursesByUser(alice.ID)
	if want := []string{second.ID, first.ID}; len(got) != 2 || got[0].ID != want[0] || got[1].ID != want[1] {
		t.Fatalf("список alice = %v, ожидали %v", favoriteIDs(got), want)
	}
	if got[0].Title != second.Title || got[0].PriceCents != second.PriceCents {
		t.Fatalf("данные курса в избранном не совпали: %+v", got[0])
	}
	if n := countFavorites(t, s, "user_id=$1::uuid", alice.ID); n != 3 {
		// Хранилище хранит и черновик (3 строки), но в список он не попадает.
		t.Fatalf("строк избранного alice: %d, ожидали 3", n)
	}

	// Чужое избранное не видно.
	if got := s.ListFavoriteCoursesByUser(bob.ID); len(got) != 0 {
		t.Fatalf("bob видит чужое избранное: %v", favoriteIDs(got))
	}

	// Архивный курс после добавления исчезает из списка.
	if err := s.AddFavorite(alice.ID, archived.ID); err != nil {
		t.Fatalf("AddFavorite(archived): %v", err)
	}
	if !s.SetCourseStatus(archived.ID, repo.CourseArchived, "") {
		t.Fatal("не удалось перевести курс в архив")
	}
	if got := s.ListFavoriteCoursesByUser(alice.ID); len(got) != 2 {
		t.Fatalf("архивный курс остался в списке: %v", favoriteIDs(got))
	}

	// Удаление идемпотентно, в т.ч. для отсутствующей пары и мусорного id.
	for i := 0; i < 2; i++ {
		if err := s.RemoveFavorite(alice.ID, second.ID); err != nil {
			t.Fatalf("RemoveFavorite #%d: %v", i+1, err)
		}
	}
	if err := s.RemoveFavorite(alice.ID, "not-a-uuid"); err != nil {
		t.Fatalf("RemoveFavorite(мусорный id): %v", err)
	}
	if got := s.ListFavoriteCoursesByUser(alice.ID); len(got) != 1 || got[0].ID != first.ID {
		t.Fatalf("после удаления список = %v, ожидали [%s]", favoriteIDs(got), first.ID)
	}

	// Несуществующие пользователь/курс — ErrNotFound (FK).
	if err := s.AddFavorite("00000000-0000-0000-0000-000000000001", first.ID); err != repo.ErrNotFound {
		t.Fatalf("AddFavorite(нет пользователя): err=%v, ожидали ErrNotFound", err)
	}
	if err := s.AddFavorite(alice.ID, "00000000-0000-0000-0000-000000000002"); err != repo.ErrNotFound {
		t.Fatalf("AddFavorite(нет курса): err=%v, ожидали ErrNotFound", err)
	}
	if err := s.AddFavorite(alice.ID, "not-a-uuid"); err != repo.ErrNotFound {
		t.Fatalf("AddFavorite(мусорный id): err=%v, ожидали ErrNotFound", err)
	}
}

// TestFavoritesCascadeParity: 013_favorites.sql удаляет строки каскадом при
// удалении курса и пользователя — как in-memory зеркало.
func TestFavoritesCascadeParity(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "t73-cascade-teacher", repo.RoleTeacher)
	alice := createUser(t, s, "t73-cascade-alice", repo.RoleStudent)
	bob := createUser(t, s, "t73-cascade-bob", repo.RoleStudent)

	courseA := createCourse(t, s, teacher.ID, "t73-cascade-a", 1000)
	courseB := createCourse(t, s, teacher.ID, "t73-cascade-b", 2000)
	for _, c := range []*repo.Course{courseA, courseB} {
		if !s.SetCourseStatus(c.ID, repo.CoursePublished, "") {
			t.Fatalf("SetCourseStatus(%s, published) не сработал", c.ID)
		}
	}
	for _, pair := range []struct {
		user   *repo.User
		course *repo.Course
	}{
		{user: alice, course: courseA}, {user: alice, course: courseB}, {user: bob, course: courseA},
	} {
		if err := s.AddFavorite(pair.user.ID, pair.course.ID); err != nil {
			t.Fatalf("AddFavorite(%s, %s): %v", pair.user.ID, pair.course.ID, err)
		}
	}
	if n := countFavorites(t, s, "course_id=$1::uuid", courseA.ID); n != 2 {
		t.Fatalf("строк по курсу A: %d, ожидали 2", n)
	}

	// Удаление курса: его строки уходят, чужие (курс B) остаются.
	if err := s.DeleteCourse(courseA.ID); err != nil {
		t.Fatalf("DeleteCourse(A): %v", err)
	}
	if n := countFavorites(t, s, "course_id=$1::uuid", courseA.ID); n != 0 {
		t.Fatalf("после удаления курса A строк: %d, ожидали 0", n)
	}
	if n := countFavorites(t, s, "user_id=$1::uuid", alice.ID); n != 1 {
		t.Fatalf("после удаления курса A у alice строк: %d, ожидали 1", n)
	}

	// Удаление пользователя: уходят все его строки, избранное bob не трогается.
	bobCourse := createCourse(t, s, teacher.ID, "t73-cascade-bob", 3000)
	if !s.SetCourseStatus(bobCourse.ID, repo.CoursePublished, "") {
		t.Fatal("не удалось опубликовать курс bob")
	}
	if err := s.AddFavorite(bob.ID, bobCourse.ID); err != nil {
		t.Fatalf("AddFavorite(bob): %v", err)
	}
	if err := s.DeleteUser(alice.ID); err != nil {
		t.Fatalf("DeleteUser(alice): %v", err)
	}
	if n := countFavorites(t, s, "user_id=$1::uuid", alice.ID); n != 0 {
		t.Fatalf("после удаления alice строк: %d, ожидали 0", n)
	}
	if n := countFavorites(t, s, "user_id=$1::uuid", bob.ID); n != 1 {
		t.Fatalf("избранное bob после удаления alice: %d, ожидали 1", n)
	}
}
