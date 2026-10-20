package postgres_test

// T71 — parity-тесты вопросов и ответов по урокам на настоящем Postgres.
// Проверяем: создание вопроса с вычисляемыми lessonTitle/studentName, ленту
// (новые первыми), фильтр по уроку и пагинацию, ответы (старые первыми) с
// authorRole/isTeacher, права store не проверяет — только принадлежность
// вопросов/ответов курсу, каскадное удаление ответов вместе с вопросом.
// Миграцию 012_lesson_questions.sql прогоняет testStore (ApplyMigrations
// падает при битом SQL/отсутствии таблиц).

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// qaFindQuestion — вопрос из ListLessonQuestions по id.
func qaFindQuestion(t *testing.T, s *repo.PostgresStore, courseID, questionID, lessonID string) *repo.LessonQuestion {
	t.Helper()
	items, _ := s.ListLessonQuestions(courseID, lessonID, 1, 100)
	for _, q := range items {
		if q.ID == questionID {
			return q
		}
	}
	t.Fatalf("вопрос %s не найден в курсе %s", questionID, courseID)
	return nil
}

// TestPostgresLessonQARoundtrip — полный жизненный цикл вопроса и ответов.
func TestPostgresLessonQARoundtrip(t *testing.T) {
	s := testStore(t)
	teacher, course, lessons := seedCourseWithLessons(t, s, 2)
	if !s.SetUserDisplayName(teacher.ID, "Преподаватель Тестов") {
		t.Fatal("не удалось задать displayName преподавателю")
	}
	student := createUser(t, s, "qa-student", repo.RoleStudent)
	if !s.SetUserDisplayName(student.ID, "Анна Студентова") {
		t.Fatal("не удалось задать displayName ученику")
	}
	admin := createUser(t, s, "qa-admin", repo.RoleAdmin)
	if !s.SetUserDisplayName(admin.ID, "Админ Проверкин") {
		t.Fatal("не удалось задать displayName администратору")
	}

	// Вопрос: вычисляемые lessonTitle/studentName, пустые ответы.
	q1, err := s.CreateLessonQuestion(course.ID, lessons[0], student.ID, "Почему переменная так называется?")
	if err != nil {
		t.Fatalf("CreateLessonQuestion: %v", err)
	}
	if q1.LessonTitle != "Урок T34 1" || q1.StudentName != "Анна Студентова" {
		t.Fatalf("вычисляемые поля вопроса: title=%q student=%q", q1.LessonTitle, q1.StudentName)
	}
	if q1.Answers == nil || len(q1.Answers) != 0 {
		t.Fatalf("answers нового вопроса=%v, ждали пустой не-nil срез", q1.Answers)
	}

	// Порядок: новые первыми; фильтр по уроку и пагинация.
	time.Sleep(2 * time.Millisecond)
	q2, err := s.CreateLessonQuestion(course.ID, lessons[1], student.ID, "А второй урок зачем нужен?")
	if err != nil {
		t.Fatalf("CreateLessonQuestion #2: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	q3, err := s.CreateLessonQuestion(course.ID, lessons[0], student.ID, "Можно пример с замыканием?")
	if err != nil {
		t.Fatalf("CreateLessonQuestion #3: %v", err)
	}

	all, total := s.ListLessonQuestions(course.ID, "", 1, 20)
	if total != 3 || len(all) != 3 {
		t.Fatalf("все вопросы: total=%d len=%d, ждали 3", total, len(all))
	}
	if all[0].ID != q3.ID || all[1].ID != q2.ID || all[2].ID != q1.ID {
		t.Fatalf("порядок вопросов нарушен: %v %v %v", all[0].ID, all[1].ID, all[2].ID)
	}
	lesson1, total1 := s.ListLessonQuestions(course.ID, lessons[0], 1, 20)
	if total1 != 2 || len(lesson1) != 2 {
		t.Fatalf("фильтр по уроку: total=%d len=%d, ждали 2", total1, len(lesson1))
	}
	for _, q := range lesson1 {
		if q.LessonID != lessons[0] {
			t.Fatalf("в фильтре чужой урок: %s", q.LessonID)
		}
	}
	page2, totalPage := s.ListLessonQuestions(course.ID, "", 2, 1)
	if totalPage != 3 || len(page2) != 1 || page2[0].ID != q2.ID {
		t.Fatalf("пагинация: total=%d items=%v, ждали q2", totalPage, page2)
	}
	// Дефолты пагинации и пустой курс — не nil.
	if items, _ := s.ListLessonQuestions(course.ID, "", 0, 0); items == nil {
		t.Fatal("ListLessonQuestions вернул nil вместо пустого среза")
	}
	if items, total := s.ListLessonQuestions(uuid.NewString(), "", 1, 20); items == nil || total != 0 {
		t.Fatalf("несуществующий курс: items=%v total=%d", items, total)
	}

	// Ответы: ученик, преподаватель, admin; роли и бейдж.
	time.Sleep(2 * time.Millisecond)
	a1, err := s.CreateLessonAnswer(course.ID, q1.ID, student.ID, "Потому что значение меняется.")
	if err != nil {
		t.Fatalf("CreateLessonAnswer(student): %v", err)
	}
	if a1.AuthorRole != "student" || a1.IsTeacher || a1.AuthorName != "Анна Студентова" {
		t.Fatalf("ответ ученика: role=%q isTeacher=%v name=%q", a1.AuthorRole, a1.IsTeacher, a1.AuthorName)
	}
	time.Sleep(2 * time.Millisecond)
	a2, err := s.CreateLessonAnswer(course.ID, q1.ID, teacher.ID, "Переменная — потому что её значение можно менять.")
	if err != nil {
		t.Fatalf("CreateLessonAnswer(teacher): %v", err)
	}
	if a2.AuthorRole != "teacher" || !a2.IsTeacher || a2.AuthorName != "Преподаватель Тестов" {
		t.Fatalf("ответ преподавателя: role=%q isTeacher=%v name=%q", a2.AuthorRole, a2.IsTeacher, a2.AuthorName)
	}
	time.Sleep(2 * time.Millisecond)
	a3, err := s.CreateLessonAnswer(course.ID, q1.ID, admin.ID, "Добавлю пример от администрации.")
	if err != nil {
		t.Fatalf("CreateLessonAnswer(admin): %v", err)
	}
	if a3.AuthorRole != "admin" || !a3.IsTeacher {
		t.Fatalf("ответ админа: role=%q isTeacher=%v", a3.AuthorRole, a3.IsTeacher)
	}

	// GetLessonQuestion: ответы старыми первыми.
	got, ok := s.GetLessonQuestion(course.ID, q1.ID)
	if !ok {
		t.Fatal("GetLessonQuestion не нашёл вопрос")
	}
	if len(got.Answers) != 3 {
		t.Fatalf("ответов в вопросе=%d, ждали 3", len(got.Answers))
	}
	if got.Answers[0].ID != a1.ID || got.Answers[1].ID != a2.ID || got.Answers[2].ID != a3.ID {
		t.Fatalf("порядок ответов нарушен: %v %v %v", got.Answers[0].ID, got.Answers[1].ID, got.Answers[2].ID)
	}
	if !got.Answers[0].CreatedAt.Before(got.Answers[2].CreatedAt) {
		t.Fatalf("время ответов не по возрастанию: %v .. %v", got.Answers[0].CreatedAt, got.Answers[2].CreatedAt)
	}
	if ans, ok := s.GetLessonAnswer(course.ID, q1.ID, a1.ID); !ok || ans.AuthorRole != "student" {
		t.Fatalf("GetLessonAnswer вернул %+v, ok=%v", ans, ok)
	}

	// Вопрос другого курса неотличим от несуществующего.
	_, otherCourse, otherLessons := seedCourseWithLessons(t, s, 1)
	if _, err := s.CreateLessonQuestion(course.ID, otherLessons[0], student.ID, "Вопрос к чужому уроку"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("CreateLessonQuestion(чужой урок): err=%v, ждали ErrNotFound", err)
	}
	if _, err := s.CreateLessonAnswer(otherCourse.ID, q1.ID, student.ID, "Ответ не туда"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("CreateLessonAnswer(чужой курс): err=%v, ждали ErrNotFound", err)
	}
	if _, ok := s.GetLessonQuestion(otherCourse.ID, q1.ID); ok {
		t.Fatal("вопрос найден по чужому курсу")
	}
	if _, ok := s.GetLessonAnswer(otherCourse.ID, q1.ID, a1.ID); ok {
		t.Fatal("ответ найден по чужому курсу")
	}

	// Удаление ответа: не тот вопрос/курс — ErrNotFound; свой — успех.
	if err := s.DeleteLessonAnswer(course.ID, q2.ID, a1.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("DeleteLessonAnswer(чужой вопрос): err=%v", err)
	}
	if err := s.DeleteLessonAnswer(otherCourse.ID, q1.ID, a1.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("DeleteLessonAnswer(чужой курс): err=%v", err)
	}
	if err := s.DeleteLessonAnswer(course.ID, q1.ID, a3.ID); err != nil {
		t.Fatalf("DeleteLessonAnswer: %v", err)
	}
	if _, ok := s.GetLessonAnswer(course.ID, q1.ID, a3.ID); ok {
		t.Fatal("удалённый ответ всё ещё читается")
	}
	if err := s.DeleteLessonAnswer(course.ID, q1.ID, a3.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("повторное удаление ответа: err=%v", err)
	}

	// Удаление вопроса — каскад ответов; повтор/чужой курс — ErrNotFound.
	if err := s.DeleteLessonQuestion(otherCourse.ID, q1.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("DeleteLessonQuestion(чужой курс): err=%v", err)
	}
	if err := s.DeleteLessonQuestion(course.ID, q1.ID); err != nil {
		t.Fatalf("DeleteLessonQuestion: %v", err)
	}
	if _, ok := s.GetLessonQuestion(course.ID, q1.ID); ok {
		t.Fatal("удалённый вопрос всё ещё читается")
	}
	if _, ok := s.GetLessonAnswer(course.ID, q1.ID, a1.ID); ok {
		t.Fatal("ответ пережил удаление вопроса (нет каскада)")
	}
	if err := s.DeleteLessonQuestion(course.ID, q1.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("повторное удаление вопроса: err=%v", err)
	}
	if err := s.DeleteLessonQuestion(course.ID, uuid.NewString()); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("DeleteLessonQuestion(нет вопроса): err=%v", err)
	}
	if err := s.DeleteLessonAnswer(course.ID, q2.ID, uuid.NewString()); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("DeleteLessonAnswer(нет ответа): err=%v", err)
	}
}
