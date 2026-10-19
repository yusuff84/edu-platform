package postgres_test

// T54 — обложка курса в Postgres-хранилище: SetCourseCover пишет и сбрасывает
// cover_path, PATCH карточки курса его не затирает. Миграцию 007_cover.sql
// прогоняет сам testStore (ApplyMigrations падает при битом SQL/нет колонки).

import (
	"encoding/json"
	"strings"
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestCourseCoverRoundtrip — запись обложки, сброс в NULL и устойчивость к
// PATCH карточки (UpdateCourse принципиально не трогает cover_path).
func TestCourseCoverRoundtrip(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "cover", repo.RoleTeacher)
	course := createCourse(t, s, teacher.ID, "cover", 99000)

	if course.CoverPath != nil {
		t.Fatalf("у нового курса coverPath=%v, ждали nil", *course.CoverPath)
	}
	// Контракт волны P7: coverPath — string|null, не пустая строка.
	bs, err := json.Marshal(course)
	if err != nil {
		t.Fatalf("marshal course: %v", err)
	}
	if !strings.Contains(string(bs), `"coverPath":null`) {
		t.Fatalf("JSON курса без coverPath:null: %s", bs)
	}

	// Хэппи-путь записи: путь привязан к курсу, как в хендлере T54.
	path := "/uploads/covers/" + course.ID + ".jpg"
	cover := path
	if !s.SetCourseCover(course.ID, &cover) {
		t.Fatal("SetCourseCover ответил false")
	}
	got, ok := s.GetCourseByID(course.ID)
	if !ok || got.CoverPath == nil || *got.CoverPath != path {
		t.Fatalf("обложка не записана: ok=%v cover=%+v", ok, got.CoverPath)
	}
	if ts, _ := json.Marshal(got); strings.Contains(string(ts), `"coverPath":""`) {
		t.Fatalf("обложка сериализовалась пустой строкой: %s", ts)
	}

	// PATCH карточки не затирает cover_path.
	title := "Курс с обложкой (обновлён)"
	if !s.UpdateCourse(course.ID, repo.CourseUpdate{Title: &title}) {
		t.Fatal("UpdateCourse ответил false")
	}
	after, ok := s.GetCourseByID(course.ID)
	if !ok || after.CoverPath == nil || *after.CoverPath != path {
		t.Fatalf("PATCH карточки затёр обложку: ok=%v cover=%+v", ok, after.CoverPath)
	}

	// Удаление обложки: cover_path → NULL → coverPath null.
	if !s.SetCourseCover(course.ID, nil) {
		t.Fatal("SetCourseCover(nil) ответил false")
	}
	removed, ok := s.GetCourseByID(course.ID)
	if !ok || removed.CoverPath != nil {
		t.Fatalf("обложка не сброшена: ok=%v cover=%+v", ok, removed.CoverPath)
	}
}
