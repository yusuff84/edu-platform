package postgres_test

// T33 + регресс приёмки qa-p4 — частичное обновление карточки курса в
// SQL-ветке хранилища.
//
// Находка приёмки P4 (поймана qa-p4.ps1 на живом стенде): динамический SET
// в PostgresStore.UpdateCourse начинал плейсхолдеры с $1, а $1 уже был
// занят id курса в WHERE id=$1::uuid. В Exec при этом уходило на один
// параметр больше, чем в запросе (лишний никогда не использовался),
// Postgres отвечал ошибкой bind, и PATCH карточки курса на Postgres
// превращался в 500 «Не удалось обновить курс». В in-memory-ветке
// (internal/repo/update.go) баг не проявляется — именно поэтому его не
// видели юнит-тесты обработчиков, работающие на in-memory сторе.
//
// Проверяем SQL-ветку: одно поле (точная форма бага), несколько полей
// сразу, отсутствие правок у чужого курса и false для несуществующего id.

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

func TestPostgresUpdateCoursePartialFields(t *testing.T) {
	s := testStore(t)
	teacher, course, _ := seedCourseWithLessons(t, s, 1)
	other := createCourse(t, s, teacher.ID, "other", 700)

	// 1) Одно поле — та самая форма: SET title=$2 … WHERE id=$1.
	newTitle := "Обновлённый T33 " + shortID()
	if !s.UpdateCourse(course.ID, repo.CourseUpdate{Title: &newTitle}) {
		t.Fatal("UpdateCourse(title): false — обновление одного поля не применилось")
	}
	got, ok := s.GetCourseByID(course.ID)
	if !ok || got.Title != newTitle {
		t.Fatalf("title не обновился: ok=%v title=%q, ждали %q", ok, got.Title, newTitle)
	}

	// 2) Несколько полей сразу: описание, slug, цена.
	newDesc := "Новое описание " + shortID()
	newSlug := uniqueSlug("upd")
	newPrice := 12345
	if !s.UpdateCourse(course.ID, repo.CourseUpdate{
		Description: &newDesc, Slug: &newSlug, PriceCents: &newPrice,
	}) {
		t.Fatal("UpdateCourse(description/slug/price): false")
	}
	got, ok = s.GetCourseByID(course.ID)
	if !ok || got.Description != newDesc || got.Slug != newSlug || got.PriceCents != newPrice {
		t.Fatalf("частичное обновление не применилось: ok=%v got=%+v", ok, got)
	}

	// 3) Чужой курс не задет: WHERE по id действительно фильтрует.
	otherGot, ok := s.GetCourseByID(other.ID)
	if !ok || otherGot.Title != other.Title || otherGot.Slug != other.Slug {
		t.Fatalf("чужой курс пострадал: ok=%v got=%+v, был %+v", ok, otherGot, other)
	}

	// 4) Несуществующий id — false (UPDATE без строк).
	missing := "00000000-0000-0000-0000-0000000000ff"
	if s.UpdateCourse(missing, repo.CourseUpdate{Title: &newTitle}) {
		t.Fatal("UpdateCourse(несуществующий id): true, ожидали false")
	}
}
