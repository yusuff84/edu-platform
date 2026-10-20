package postgres_test

// Расширения P9 (20.10.2026): parity-тесты SQL-ветки для правки Q&A,
// массового признака isFavorite и fallback похожих курсов. Env-gated через
// TEST_DATABASE_URL, как остальные тесты пакета.

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// TestPostgresQAEditParity — правка вопроса/ответа в SQL-ветке: обновлённый
// текст читается, чужой курс/несуществующий id дают ErrNotFound, удаление
// вопроса каскадно убирает ответ.
func TestPostgresQAEditParity(t *testing.T) {
	s := testStore(t)
	teacher, course, lessons := seedCourseWithLessons(t, s, 2)
	student := createUser(t, s, "qa-edit-student", repo.RoleStudent)

	q, err := s.CreateLessonQuestion(course.ID, lessons[0], student.ID, "Исходный вопрос по уроку")
	if err != nil {
		t.Fatalf("CreateLessonQuestion: %v", err)
	}
	if q.ID == "" || q.Body != "Исходный вопрос по уроку" {
		t.Fatalf("вопрос не сохранён: %+v", q)
	}
	a, err := s.CreateLessonAnswer(course.ID, q.ID, teacher.ID, "Исходный ответ преподавателя")
	if err != nil {
		t.Fatalf("CreateLessonAnswer: %v", err)
	}
	if a.ID == "" || a.Body != "Исходный ответ преподавателя" {
		t.Fatalf("ответ не сохранён: %+v", a)
	}

	updatedQ, err := s.UpdateLessonQuestionBody(course.ID, q.ID, "Новый текст вопроса")
	if err != nil || updatedQ.Body != "Новый текст вопроса" {
		t.Fatalf("UpdateLessonQuestionBody: err=%v q=%+v", err, updatedQ)
	}
	updatedA, err := s.UpdateLessonAnswerBody(course.ID, q.ID, a.ID, "Новый текст ответа")
	if err != nil || updatedA.Body != "Новый текст ответа" {
		t.Fatalf("UpdateLessonAnswerBody: err=%v a=%+v", err, updatedA)
	}

	gotQ, ok := s.GetLessonQuestion(course.ID, q.ID)
	if !ok || gotQ.Body != "Новый текст вопроса" || len(gotQ.Answers) != 1 {
		t.Fatalf("GetLessonQuestion после правки: ok=%v q=%+v", ok, gotQ)
	}
	if gotQ.Answers[0].Body != "Новый текст ответа" {
		t.Fatalf("лента не отдала новый ответ: %+v", gotQ.Answers[0])
	}

	other := createCourse(t, s, teacher.ID, "qa-edit-other", 1000)
	if _, err := s.UpdateLessonQuestionBody(other.ID, q.ID, "Чужой курс"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("правка вопроса через чужой курс: err=%v, ожидали ErrNotFound", err)
	}
	if _, err := s.UpdateLessonAnswerBody(other.ID, q.ID, a.ID, "Чужой курс"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("правка ответа через чужой курс: err=%v, ожидали ErrNotFound", err)
	}
	if _, err := s.UpdateLessonQuestionBody(course.ID, "not-a-uuid", "Мусорный id"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("правка вопроса с мусорным id: err=%v, ожидали ErrNotFound", err)
	}

	if err := s.DeleteLessonQuestion(course.ID, q.ID); err != nil {
		t.Fatalf("DeleteLessonQuestion: %v", err)
	}
	if _, ok := s.GetLessonAnswer(course.ID, q.ID, a.ID); ok {
		t.Fatal("ответ остался после каскадного удаления вопроса")
	}
}

// TestPostgresFavoriteCourseIDs — массовый признак избранного для публичных
// видов курса: возвращаются все курсы пользователя (включая черновик, как в
// in-memory), чужое избранное не видно, мусорный id — пустая не-nil карта.
func TestPostgresFavoriteCourseIDs(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "favorite-ids-teacher", repo.RoleTeacher)
	alice := createUser(t, s, "favorite-ids-alice", repo.RoleStudent)
	bob := createUser(t, s, "favorite-ids-bob", repo.RoleStudent)

	first := createCourse(t, s, teacher.ID, "favorite-ids-first", 1000)
	second := createCourse(t, s, teacher.ID, "favorite-ids-second", 2000)
	draft := createCourse(t, s, teacher.ID, "favorite-ids-draft", 3000)
	for _, c := range []*repo.Course{first, second} {
		if !s.SetCourseStatus(c.ID, repo.CoursePublished, "") {
			t.Fatalf("SetCourseStatus(%s): false", c.ID)
		}
	}
	for _, id := range []string{first.ID, second.ID, draft.ID} {
		if err := s.AddFavorite(alice.ID, id); err != nil {
			t.Fatalf("AddFavorite(%s): %v", id, err)
		}
	}

	ids := s.FavoriteCourseIDs(alice.ID)
	if ids == nil || len(ids) != 3 {
		t.Fatalf("FavoriteCourseIDs(alice)=%v, ожидали 3 элемента", ids)
	}
	for _, id := range []string{first.ID, second.ID, draft.ID} {
		if !ids[id] {
			t.Fatalf("в FavoriteCourseIDs нет курса %s: %v", id, ids)
		}
	}
	if got := s.FavoriteCourseIDs(bob.ID); got == nil || len(got) != 0 {
		t.Fatalf("FavoriteCourseIDs(bob)=%v, ожидали пустую не-nil карту", got)
	}
	if got := s.FavoriteCourseIDs("not-a-uuid"); got == nil || len(got) != 0 {
		t.Fatalf("FavoriteCourseIDs(мусор)=%v, ожидали пустую не-nil карту", got)
	}
}

// TestPostgresSimilarCoursesEmptyCategoryFallback — при пустой категории
// подборка добирается published-курсами других категорий; при непустой
// категории по-прежнему отдаются только курсы той же категории.
func TestPostgresSimilarCoursesEmptyCategoryFallback(t *testing.T) {
	s := testStore(t)
	teacher := createUser(t, s, "similar-fallback", repo.RoleTeacher)
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)

	emptyCategory := fmt.Sprintf("empty-%s", shortID())
	src := publishSimilarCourse(t, s, teacher.ID, "fallback-src", emptyCategory, base)

	otherCategory := fmt.Sprintf("other-%s", shortID())
	other := publishSimilarCourse(t, s, teacher.ID, "fallback-other", otherCategory, base.Add(time.Hour))
	enrollSimilarCourse(t, s, other.ID, 10)
	rateSimilarCourse(t, s, other.ID, 5)

	got := s.ListSimilarCourses(src.ID, 12)
	foundOther := false
	for _, c := range got {
		if c.ID == other.ID {
			foundOther = true
		}
		if c.Category == emptyCategory {
			t.Fatalf("fallback вернул курс пустой категории: %s", c.Slug)
		}
		if c.Status != repo.CoursePublished {
			t.Fatalf("fallback вернул не published: %s (%s)", c.Slug, c.Status)
		}
	}
	if !foundOther {
		t.Fatalf("fallback не добрал курс другой категории %s: %v", other.ID, similarIDs(got...))
	}

	// Непустая категория: fallback не должен подмешивать чужие категории.
	sameCategory := fmt.Sprintf("same-%s", shortID())
	src2 := publishSimilarCourse(t, s, teacher.ID, "same-src", sameCategory, base)
	same := publishSimilarCourse(t, s, teacher.ID, "same-course", sameCategory, base.Add(time.Hour))
	foreignCategory := fmt.Sprintf("foreign-%s", shortID())
	foreign := publishSimilarCourse(t, s, teacher.ID, "same-foreign", foreignCategory, base.Add(2*time.Hour))
	enrollSimilarCourse(t, s, same.ID, 3)
	enrollSimilarCourse(t, s, foreign.ID, 10)

	for _, c := range s.ListSimilarCourses(src2.ID, 12) {
		if c.Category != sameCategory {
			t.Fatalf("при непустой категории подмешан курс %s (%s)", c.Slug, c.Category)
		}
		if c.ID == foreign.ID {
			t.Fatal("при непустой категории вернулся курс другой категории")
		}
	}
}
