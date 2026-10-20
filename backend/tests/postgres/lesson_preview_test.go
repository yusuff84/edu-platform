package postgres_test

// T75 (P9): parity SQL-ветки для флага isPreview. Проверяем миграцию 014
// (колонка is_preview NOT NULL DEFAULT false), запись при CreateLesson,
// чтение во всех выборках уроков (GetLesson/ListLessonsByModule/
// ListLessonsByCourse), UpdateLesson и клонирование курса. Env-gated как
// остальные тесты пакета: без TEST_DATABASE_URL — t.Skip.

import (
	"context"
	"testing"

	"edu-platform/backend/internal/repo"
)

func TestPostgresLessonPreviewRoundtrip(t *testing.T) {
	s := testStore(t)

	// Два урока из общего сида: по умолчанию (миграция/DEFAULT) — не превью.
	_, course, ids := seedCourseWithLessons(t, s, 2)
	for _, id := range ids {
		l, ok := s.GetLesson(id)
		if !ok {
			t.Fatalf("GetLesson(%s): урок не найден", id)
		}
		if l.IsPreview {
			t.Fatalf("урок %s создан превью без флага: %+v", id, l)
		}
	}

	// CreateLesson с isPreview=true: флаг возвращается и читается из БД.
	preview := s.CreateLesson(&repo.Lesson{
		ModuleID:  idsModuleID(t, s, ids[0]),
		Title:     "Превью-урок T75",
		Kind:      repo.LessonVideoText,
		BodyMD:    "текст превью",
		IsPreview: true,
	})
	if !preview.IsPreview {
		t.Fatalf("CreateLesson не вернул isPreview=true: %+v", preview)
	}
	got, ok := s.GetLesson(preview.ID)
	if !ok || !got.IsPreview {
		t.Fatalf("GetLesson после создания: ok=%v got=%+v", ok, got)
	}

	// Прямой SELECT: колонка реально есть в схеме и хранит значение
	// (миграция 014 применилась в SQL-ветке).
	var raw bool
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT is_preview FROM lessons WHERE id=$1::uuid`, preview.ID).Scan(&raw); err != nil {
		t.Fatalf("SELECT is_preview: %v", err)
	}
	if !raw {
		t.Fatal("в БД is_preview=false, ожидали true")
	}

	// Все выборки уроков отдают флаг: модуль и курс.
	byModule := s.ListLessonsByModule(preview.ModuleID)
	if !hasPreviewFlag(byModule, preview.ID, true) {
		t.Fatalf("ListLessonsByModule: у %s нет isPreview=true: %+v", preview.ID, byModule)
	}
	byCourse := s.ListLessonsByCourse(course.ID)
	if !hasPreviewFlag(byCourse, preview.ID, true) {
		t.Fatalf("ListLessonsByCourse: у %s нет isPreview=true: %+v", preview.ID, byCourse)
	}

	// UpdateLesson переключает флаг в обе стороны.
	l, _ := s.GetLesson(ids[0])
	l.IsPreview = true
	if err := s.UpdateLesson(l); err != nil {
		t.Fatalf("UpdateLesson(true): %v", err)
	}
	if got, _ := s.GetLesson(ids[0]); !got.IsPreview {
		t.Fatalf("UpdateLesson не сохранил isPreview=true: %+v", got)
	}
	l.IsPreview = false
	if err := s.UpdateLesson(l); err != nil {
		t.Fatalf("UpdateLesson(false): %v", err)
	}
	if got, _ := s.GetLesson(ids[0]); got.IsPreview {
		t.Fatalf("UpdateLesson не сохранил isPreview=false: %+v", got)
	}

	// Клонирование курса сохраняет флаг превью у уроков копии.
	clone, err := s.CloneCourse(course.ID)
	if err != nil {
		t.Fatalf("CloneCourse: %v", err)
	}
	cloneLessons := s.ListLessonsByCourse(clone.ID)
	if len(cloneLessons) != 3 {
		t.Fatalf("в копии %d уроков, ожидали 3: %+v", len(cloneLessons), cloneLessons)
	}
	clonePreview := false
	for _, cl := range cloneLessons {
		if cl.Title == "Превью-урок T75" {
			clonePreview = cl.IsPreview
		}
	}
	if !clonePreview {
		t.Fatalf("копия курса потеряла isPreview=true: %+v", cloneLessons)
	}
}

// idsModuleID возвращает moduleId урока — нужен, чтобы создать ещё один урок
// в том же модуле, не завязываясь на внутренние поля сида.
func idsModuleID(t *testing.T, s *repo.PostgresStore, lessonID string) string {
	t.Helper()
	l, ok := s.GetLesson(lessonID)
	if !ok {
		t.Fatalf("GetLesson(%s): урок не найден", lessonID)
	}
	return l.ModuleID
}

// hasPreviewFlag ищет урок по id и сверяет флаг.
func hasPreviewFlag(lessons []*repo.Lesson, id string, want bool) bool {
	for _, l := range lessons {
		if l.ID == id {
			return l.IsPreview == want
		}
	}
	return false
}
