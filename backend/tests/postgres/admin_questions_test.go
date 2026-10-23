package postgres_test

// T85 на Postgres: глобальный список Q&A для админки (env-gated через
// TEST_DATABASE_URL, как остальные тесты пакета). Проверяются join-поля
// курса/урока/студента/авторов ответов, фильтры courseId/lessonId/q,
// сортировка вопросов и ответов, пагинация/total, не-nil answers и
// устойчивость к мусорным UUID (handler валидирует их раньше — 400).

import (
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestPostgresAdminQuestionsParity — SQL-ветка ListAdminQuestions.
func TestPostgresAdminQuestionsParity(t *testing.T) {
	s := testStore(t)

	teacher, course, lessons := seedCourseWithLessons(t, s, 2)
	if !s.SetUserDisplayName(teacher.ID, "Галина Наставник") {
		t.Fatal("не удалось задать displayName учителю")
	}
	anna := createUser(t, s, "t85-anna", repo.RoleStudent)
	boris := createUser(t, s, "t85-boris", repo.RoleStudent)
	if !s.SetUserDisplayName(anna.ID, "Анна Петрова") {
		t.Fatal("не удалось задать displayName Анне")
	}
	if !s.SetUserDisplayName(boris.ID, "Борис Смирнов") {
		t.Fatal("не удалось задать displayName Борису")
	}

	other := createCourse(t, s, teacher.ID, "t85-sql", 0)
	otherModule := s.CreateModule(other.ID, "Модуль SQL")
	otherLesson := s.CreateLesson(&repo.Lesson{
		ModuleID: otherModule.ID,
		Title:    "Урок про индексы",
		Kind:     repo.LessonVideoText,
		BodyMD:   "текст урока",
	})
	if otherLesson == nil || otherLesson.ID == "" {
		t.Fatal("CreateLesson для второго курса не вернул урок")
	}

	q1, err := s.CreateLessonQuestion(course.ID, lessons[0], anna.ID,
		"Почему переменная так называется?")
	if err != nil {
		t.Fatalf("CreateLessonQuestion q1: %v", err)
	}
	q2, err := s.CreateLessonQuestion(course.ID, lessons[1], anna.ID,
		"Как работает цикл for?")
	if err != nil {
		t.Fatalf("CreateLessonQuestion q2: %v", err)
	}
	q3, err := s.CreateLessonQuestion(other.ID, otherLesson.ID, boris.ID,
		"Зачем нужен индекс в базе?")
	if err != nil {
		t.Fatalf("CreateLessonQuestion q3: %v", err)
	}

	if _, err := s.CreateLessonAnswer(course.ID, q1.ID, teacher.ID,
		"Переменная — это именованная ячейка памяти."); err != nil {
		t.Fatalf("CreateLessonAnswer a1: %v", err)
	}
	if _, err := s.CreateLessonAnswer(course.ID, q2.ID, anna.ID,
		"Цикл повторяет блок кода."); err != nil {
		t.Fatalf("CreateLessonAnswer a2: %v", err)
	}
	if _, err := s.CreateLessonAnswer(course.ID, q2.ID, anna.ID,
		"Цикл for состоит из трёх частей."); err != nil {
		t.Fatalf("CreateLessonAnswer a3: %v", err)
	}

	// Весь курс: новые вопросы первыми (q2 создан после q1).
	items, total := s.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: course.ID, Page: 1, Limit: 20,
	})
	if total != 2 || len(items) != 2 {
		t.Fatalf("courseId: total=%d items=%d, ждали 2/2", total, len(items))
	}
	if items[0].ID != q2.ID || items[1].ID != q1.ID {
		t.Fatalf("порядок вопросов: %s,%s, ждали %s,%s",
			items[0].ID, items[1].ID, q2.ID, q1.ID)
	}

	// Поля строки q1: курс, урок, студент, автор ответа.
	if items[1].CourseID != course.ID || items[1].CourseTitle != course.Title ||
		items[1].CourseSlug != course.Slug {
		t.Fatalf("поля курса q1: %+v", items[1])
	}
	if items[1].LessonID != lessons[0] || items[1].LessonTitle != "Урок T34 1" {
		t.Fatalf("поля урока q1: %+v", items[1])
	}
	if items[1].StudentID != anna.ID || items[1].StudentEmail != anna.Email ||
		items[1].StudentName != "Анна Петрова" || items[1].CreatedAt.IsZero() {
		t.Fatalf("поля студента q1: %+v", items[1])
	}
	if len(items[1].Answers) != 1 {
		t.Fatalf("у q1 ответов %d, ждали 1", len(items[1].Answers))
	}
	ans := items[1].Answers[0]
	if ans.AuthorID != teacher.ID || ans.AuthorEmail != teacher.Email ||
		ans.AuthorName != "Галина Наставник" || ans.AuthorRole != "teacher" ||
		ans.Body != "Переменная — это именованная ячейка памяти." {
		t.Fatalf("поля ответа q1: %+v", ans)
	}

	// Ответы q2 — старые первыми.
	if len(items[0].Answers) != 2 {
		t.Fatalf("у q2 ответов %d, ждали 2", len(items[0].Answers))
	}
	if items[0].Answers[0].Body != "Цикл повторяет блок кода." ||
		items[0].Answers[1].Body != "Цикл for состоит из трёх частей." {
		t.Fatalf("порядок ответов q2: %+v", items[0].Answers)
	}

	// Фильтр lessonId.
	byLesson, byLessonTotal := s.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: course.ID, LessonID: lessons[1], Page: 1, Limit: 20,
	})
	if byLessonTotal != 1 || len(byLesson) != 1 || byLesson[0].ID != q2.ID {
		t.Fatalf("фильтр lessonId: total=%d items=%+v", byLessonTotal, byLesson)
	}

	// Поиск без учёта регистра по вопросу, ответу, автору ответа, ученику,
	// названиям урока и курса.
	searches := []struct {
		name string
		f    repo.AdminQuestionFilter
		want []string
	}{
		{"по вопросу", repo.AdminQuestionFilter{CourseID: course.ID, Q: "ПЕРЕМЕННАЯ"}, []string{q1.ID}},
		{"по ответу", repo.AdminQuestionFilter{CourseID: course.ID, Q: "именованная"}, []string{q1.ID}},
		{"по автору ответа", repo.AdminQuestionFilter{CourseID: course.ID, Q: "галина"}, []string{q1.ID}},
		{"по имени ученика", repo.AdminQuestionFilter{CourseID: course.ID, Q: "анна"}, []string{q2.ID, q1.ID}},
		{"по названию урока", repo.AdminQuestionFilter{CourseID: course.ID, Q: "T34 2"}, []string{q2.ID}},
		{"по названию курса", repo.AdminQuestionFilter{CourseID: other.ID, Q: "sql"}, []string{q3.ID}},
		{"нет совпадений", repo.AdminQuestionFilter{CourseID: course.ID, Q: "нет-такого-текста"}, nil},
	}
	for _, tc := range searches {
		tc.f.Page, tc.f.Limit = 1, 20
		got, gotTotal := s.ListAdminQuestions(tc.f)
		if gotTotal != len(tc.want) || len(got) != len(tc.want) {
			t.Fatalf("поиск %s: total=%d items=%d, ждали %d",
				tc.name, gotTotal, len(got), len(tc.want))
		}
		for i := range tc.want {
			if got[i].ID != tc.want[i] {
				t.Fatalf("поиск %s: порядок %v, ждали %v", tc.name, t85PGIDs(got), tc.want)
			}
		}
	}

	// Пагинация: total до limit.
	page2, pageTotal := s.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: course.ID, Page: 2, Limit: 1,
	})
	if pageTotal != 2 || len(page2) != 1 || page2[0].ID != q1.ID {
		t.Fatalf("пагинация: total=%d items=%+v", pageTotal, page2)
	}
	beyond, beyondTotal := s.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: course.ID, Page: 5, Limit: 1,
	})
	if beyondTotal != 2 || beyond == nil || len(beyond) != 0 {
		t.Fatalf("страница за пределами: total=%d items=%#v", beyondTotal, beyond)
	}
	empty, emptyTotal := s.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: course.ID, Q: "нет-такого-текста", Page: 1, Limit: 20,
	})
	if emptyTotal != 0 || empty == nil || len(empty) != 0 {
		t.Fatalf("пустая выборка должна быть [] (не nil): total=%d items=%#v", emptyTotal, empty)
	}

	// Другой курс и пустые answers — не nil.
	otherItems, otherTotal := s.ListAdminQuestions(repo.AdminQuestionFilter{
		CourseID: other.ID, Page: 1, Limit: 20,
	})
	if otherTotal != 1 || len(otherItems) != 1 || otherItems[0].ID != q3.ID {
		t.Fatalf("второй курс: total=%d items=%+v", otherTotal, otherItems)
	}
	if otherItems[0].Answers == nil || len(otherItems[0].Answers) != 0 {
		t.Fatalf("answers без ответов должен быть пустым массивом: %#v", otherItems[0].Answers)
	}

	// Мусорные UUID — пустая выборка без SQL-ошибки.
	for _, f := range []repo.AdminQuestionFilter{
		{CourseID: "not-a-uuid"},
		{LessonID: "not-a-uuid"},
	} {
		got, gotTotal := s.ListAdminQuestions(f)
		if gotTotal != 0 || got == nil || len(got) != 0 {
			t.Fatalf("мусорный UUID: total=%d items=%#v", gotTotal, got)
		}
	}
}

// t85PGIDs — id вопросов в порядке выдачи (для сообщений об ошибке).
func t85PGIDs(items []*repo.AdminQuestionRow) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
