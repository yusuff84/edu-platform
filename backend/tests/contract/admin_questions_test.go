package contract_test

// T85 — глобальная модерация Q&A (волна P10).
// Контракт — Task/00-api-contract.md, раздел «T85 — GET /admin/questions»
// (заморожен):
//   - только admin: 401 без сессии, 403 teacher/student;
//   - фильтры courseId/lessonId (UUID), q (без учёта регистра по вопросам,
//     ответам, email/именам, названиям курса и урока), page/limit;
//   - новые вопросы первыми, ответы внутри — старыми первыми;
//   - total до пагинации, items всегда массив, answers всегда массив;
//   - неверные UUID/пагинация — 400;
//   - удаление чужих вопросов/ответов — существующими маршрутами T71,
//     admin может, посторонний teacher — нет.
//
// SQL-ветка проверяется отдельно на живом Postgres —
// backend/tests/postgres/admin_questions_test.go.

import (
	"net/http"
	"net/url"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// t85Page — конверт GET /admin/questions в типизированном виде: заодно
// проверяет JSON-теги repo.AdminQuestionRow/AdminAnswerRow.
type t85Page struct {
	Items []*repo.AdminQuestionRow `json:"items"`
	Total int                      `json:"total"`
	Page  int                      `json:"page"`
	Limit int                      `json:"limit"`
}

// t85Fixture — общая подготовка: admin, владелец курсов, посторонний
// teacher и два ученика; два published-курса с уроками и активными записями.
type t85Fixture struct {
	app    *handler.App
	router http.Handler

	adminCookies        []*http.Cookie
	teacherCookies      []*http.Cookie
	otherTeacherCookies []*http.Cookie
	annaCookies         []*http.Cookie
	borisCookies        []*http.Cookie

	teacherID string
	annaID    string
	borisID   string

	course1ID   string
	course1Slug string
	course2ID   string
	course2Slug string

	lesson1ID string
	lesson2ID string
	lesson3ID string
}

// t85CreateCourse создаёт черновик через API учителя.
func t85CreateCourse(t *testing.T, router http.Handler, cookies []*http.Cookie, title string) (id, slug string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": title, "description": "Описание курса T85", "priceCents": 0,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var course map[string]any
	decodeBody(t, rec, &course)
	id, _ = course["id"].(string)
	slug, _ = course["slug"].(string)
	if id == "" || slug == "" {
		t.Fatalf("create course %q: нет id/slug: %s", title, rec.Body.String())
	}
	return id, slug
}

// t85AddLesson добавляет video_text-урок в первый модуль курса (модуль
// создаётся, если его ещё нет).
func t85AddLesson(t *testing.T, app *handler.App, router http.Handler, cookies []*http.Cookie, courseID, title string) string {
	t.Helper()
	modules := app.Store.ListModulesByCourse(courseID)
	if len(modules) == 0 {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
			"title": "Модуль T85",
		}, cookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
		}
		modules = app.Store.ListModulesByCourse(courseID)
	}
	if len(modules) == 0 {
		t.Fatalf("у курса %s нет модулей после создания", courseID)
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": modules[0].ID, "title": title, "kind": "video_text", "bodyMd": "текст урока T85",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	id, _ := lesson["id"].(string)
	if id == "" {
		t.Fatalf("create lesson %q: нет id в ответе: %s", title, rec.Body.String())
	}
	return id
}

// t85Seed регистрирует пользователей (роли — напрямую в store, как в других
// контрактных тестах), создаёт два опубликованных курса и записывает учеников.
func t85Seed(t *testing.T) *t85Fixture {
	t.Helper()
	app, router := newTestApp()
	fx := &t85Fixture{app: app, router: router}

	fx.adminCookies, _ = registerUser(t, router, "t85-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "t85-admin@example.com")

	fx.teacherCookies, fx.teacherID = registerUser(t, router, "t85-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(fx.teacherID, repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	if !app.Store.SetUserDisplayName(fx.teacherID, "Галина Наставник") {
		t.Fatal("не удалось задать displayName учителю")
	}

	var otherTeacherID string
	fx.otherTeacherCookies, otherTeacherID = registerUser(t, router, "t85-other-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(otherTeacherID, repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher второму учителю")
	}
	if !app.Store.SetUserDisplayName(otherTeacherID, "Другой Преподаватель") {
		t.Fatal("не удалось задать displayName второму учителю")
	}

	fx.annaCookies, fx.annaID = registerUser(t, router, "t85-anna@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(fx.annaID, "Анна Петрова") {
		t.Fatal("не удалось задать displayName Анне")
	}
	fx.borisCookies, fx.borisID = registerUser(t, router, "t85-boris@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(fx.borisID, "Борис Смирнов") {
		t.Fatal("не удалось задать displayName Борису")
	}

	fx.course1ID, fx.course1Slug = t85CreateCourse(t, router, fx.teacherCookies, "Курс по Go")
	fx.lesson1ID = t85AddLesson(t, app, router, fx.teacherCookies, fx.course1ID, "Урок про переменные")
	fx.lesson2ID = t85AddLesson(t, app, router, fx.teacherCookies, fx.course1ID, "Урок про циклы")

	fx.course2ID, fx.course2Slug = t85CreateCourse(t, router, fx.teacherCookies, "Курс по SQL")
	fx.lesson3ID = t85AddLesson(t, app, router, fx.teacherCookies, fx.course2ID, "Урок про индексы")

	for _, id := range []string{fx.course1ID, fx.course2ID} {
		if !app.Store.SetCourseStatus(id, repo.CoursePublished, "") {
			t.Fatalf("не удалось опубликовать курс %s", id)
		}
	}
	for _, pair := range []struct {
		cookies  []*http.Cookie
		courseID string
	}{
		{fx.annaCookies, fx.course1ID},
		{fx.borisCookies, fx.course2ID},
	} {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
			"courseId": pair.courseID,
		}, pair.cookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	return fx
}

// t85PostAnswer создаёт ответ на вопрос через API T71.
func t85PostAnswer(t *testing.T, router http.Handler, courseID, questionID, body string, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/courses/"+courseID+"/questions/"+questionID+"/answers",
		map[string]any{"body": body}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create answer: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	answer, _ := out["answer"].(map[string]any)
	if answer == nil {
		t.Fatalf("в ответе нет answer: %s", rec.Body.String())
	}
	return answer
}

// t85Fetch вызывает GET /admin/questions и ожидает 200.
func t85Fetch(t *testing.T, router http.Handler, cookies []*http.Cookie, pairs map[string]string) *t85Page {
	t.Helper()
	q := url.Values{}
	for k, v := range pairs {
		q.Set(k, v)
	}
	path := "/api/v1/admin/questions"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	rec := doRequest(t, router, http.MethodGet, path, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	var page t85Page
	decodeBody(t, rec, &page)
	if page.Items == nil {
		t.Fatalf("items должны быть массивом, не null: %s", rec.Body.String())
	}
	return &page
}

// t85IDs — id вопросов в порядке выдачи.
func t85IDs(items []*repo.AdminQuestionRow) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// t85WantIDs сравнивает порядок id.
func t85WantIDs(t *testing.T, got, want []string, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: id=%v, ждали %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: порядок %v, ждали %v", what, got, want)
		}
	}
}

// TestAdminQuestionsGlobalList — пустой список, фильтры, поиск, сортировка,
// пагинация/total, форма ответа, 400/401/403.
func TestAdminQuestionsGlobalList(t *testing.T) {
	fx := t85Seed(t)

	// Пустой список: items — [], page/limit — дефолты.
	empty := t85Fetch(t, fx.router, fx.adminCookies, nil)
	if empty.Total != 0 || len(empty.Items) != 0 || empty.Page != 1 || empty.Limit != 20 {
		t.Fatalf("пустой список: %+v", empty)
	}

	// Вопросы создаются в порядке q1, q2, q3 — новые первыми ожидаем q3,q2,q1.
	q1 := qaPostQuestion(t, fx.router, fx.course1ID, fx.lesson1ID,
		"Почему переменная так называется?", fx.annaCookies)
	q2 := qaPostQuestion(t, fx.router, fx.course1ID, fx.lesson2ID,
		"Как работает цикл for?", fx.annaCookies)
	q3 := qaPostQuestion(t, fx.router, fx.course2ID, fx.lesson3ID,
		"Зачем нужен индекс в базе?", fx.borisCookies)
	q1ID, _ := q1["id"].(string)
	q2ID, _ := q2["id"].(string)
	q3ID, _ := q3["id"].(string)
	if q1ID == "" || q2ID == "" || q3ID == "" {
		t.Fatalf("вопросы без id: q1=%v q2=%v q3=%v", q1, q2, q3)
	}

	// Ответы: у q1 — преподаватель, у q2 — два ответа ученицы (порядок
	// «старые первыми»), у q3 ответов нет.
	a1 := t85PostAnswer(t, fx.router, fx.course1ID, q1ID,
		"Переменная — это именованная ячейка памяти.", fx.teacherCookies)
	if _, ok := a1["id"].(string); !ok {
		t.Fatalf("ответ без id: %v", a1)
	}
	t85PostAnswer(t, fx.router, fx.course1ID, q2ID,
		"Цикл повторяет блок кода.", fx.annaCookies)
	t85PostAnswer(t, fx.router, fx.course1ID, q2ID,
		"Цикл for состоит из трёх частей.", fx.annaCookies)

	// Весь список: новые вопросы первыми; пустых answers нет.
	all := t85Fetch(t, fx.router, fx.adminCookies, nil)
	if all.Total != 3 {
		t.Fatalf("total=%d, ждали 3", all.Total)
	}
	t85WantIDs(t, t85IDs(all.Items), []string{q3ID, q2ID, q1ID}, "порядок вопросов")
	if all.Items[0].Answers == nil || len(all.Items[0].Answers) != 0 {
		t.Fatalf("у q3 answers должен быть пустым массивом: %#v", all.Items[0].Answers)
	}

	// Поля строки q1: курс/урок/студент/автор ответа.
	var row1 *repo.AdminQuestionRow
	for _, item := range all.Items {
		if item.ID == q1ID {
			row1 = item
		}
	}
	if row1 == nil {
		t.Fatalf("q1 нет в списке: %+v", t85IDs(all.Items))
	}
	if row1.CourseID != fx.course1ID || row1.CourseTitle != "Курс по Go" ||
		row1.CourseSlug != fx.course1Slug {
		t.Fatalf("поля курса q1: %+v", row1)
	}
	if row1.LessonID != fx.lesson1ID || row1.LessonTitle != "Урок про переменные" {
		t.Fatalf("поля урока q1: %+v", row1)
	}
	if row1.StudentID != fx.annaID || row1.StudentEmail != "t85-anna@example.com" ||
		row1.StudentName != "Анна Петрова" || row1.Body != "Почему переменная так называется?" ||
		row1.CreatedAt.IsZero() {
		t.Fatalf("поля студента/вопроса q1: %+v", row1)
	}
	if len(row1.Answers) != 1 {
		t.Fatalf("у q1 ответов %d, ждали 1", len(row1.Answers))
	}
	ans := row1.Answers[0]
	if ans.AuthorID != fx.teacherID || ans.AuthorEmail != "t85-teacher@example.com" ||
		ans.AuthorName != "Галина Наставник" || ans.AuthorRole != "teacher" ||
		ans.Body != "Переменная — это именованная ячейка памяти." || ans.CreatedAt.IsZero() {
		t.Fatalf("поля ответа q1: %+v", ans)
	}

	// Ответы q2 — старые первыми.
	var row2 *repo.AdminQuestionRow
	for _, item := range all.Items {
		if item.ID == q2ID {
			row2 = item
		}
	}
	if row2 == nil || len(row2.Answers) != 2 {
		t.Fatalf("у q2 ожидали 2 ответа: %+v", row2)
	}
	if row2.Answers[0].Body != "Цикл повторяет блок кода." ||
		row2.Answers[1].Body != "Цикл for состоит из трёх частей." {
		t.Fatalf("порядок ответов q2: %+v", row2.Answers)
	}

	// Фильтры courseId/lessonId.
	course1 := t85Fetch(t, fx.router, fx.adminCookies, map[string]string{"courseId": fx.course1ID})
	if course1.Total != 2 {
		t.Fatalf("courseId: total=%d, ждали 2", course1.Total)
	}
	t85WantIDs(t, t85IDs(course1.Items), []string{q2ID, q1ID}, "фильтр courseId")
	lesson2 := t85Fetch(t, fx.router, fx.adminCookies, map[string]string{
		"courseId": fx.course1ID, "lessonId": fx.lesson2ID,
	})
	if lesson2.Total != 1 || len(lesson2.Items) != 1 || lesson2.Items[0].ID != q2ID {
		t.Fatalf("фильтр lessonId: %+v", lesson2)
	}

	// Поиск без учёта регистра: вопрос, ответ, автор ответа, ученик, названия
	// урока и курса.
	searches := []struct {
		name string
		q    string
		want []string
	}{
		{"по вопросу", "ПЕРЕМЕННАЯ", []string{q1ID}},
		{"по ответу", "именованная", []string{q1ID}},
		{"по имени автора ответа", "галина", []string{q1ID}},
		{"по имени ученика", "анна", []string{q2ID, q1ID}},
		{"по email ученика", "t85-anna@", []string{q2ID, q1ID}},
		{"по названию урока", "циклы", []string{q2ID}},
		{"по названию курса", "sql", []string{q3ID}},
		{"нет совпадений", "нет-такого-текста", nil},
	}
	for _, tc := range searches {
		got := t85Fetch(t, fx.router, fx.adminCookies, map[string]string{"q": tc.q})
		t85WantIDs(t, t85IDs(got.Items), tc.want, "поиск: "+tc.name)
		if got.Total != len(tc.want) {
			t.Fatalf("поиск %q: total=%d, ждали %d", tc.q, got.Total, len(tc.want))
		}
	}

	// Пагинация: total до limit, page/limit эхом в ответе.
	page2 := t85Fetch(t, fx.router, fx.adminCookies, map[string]string{"page": "2", "limit": "2"})
	if page2.Total != 3 || page2.Page != 2 || page2.Limit != 2 {
		t.Fatalf("пагинация page=2 limit=2: %+v", page2)
	}
	t85WantIDs(t, t85IDs(page2.Items), []string{q1ID}, "пагинация page=2 limit=2")
	beyond := t85Fetch(t, fx.router, fx.adminCookies, map[string]string{"page": "5", "limit": "2"})
	if beyond.Total != 3 || len(beyond.Items) != 0 {
		t.Fatalf("страница за пределами: %+v", beyond)
	}

	// Неверные UUID/пагинация — 400.
	badQueries := []string{
		"courseId=not-a-uuid",
		"lessonId=not-a-uuid",
		"page=0",
		"page=-1",
		"page=abc",
		"limit=0",
		"limit=101",
		"limit=abc",
	}
	for _, query := range badQueries {
		rec := doRequest(t, fx.router, http.MethodGet, "/api/v1/admin/questions?"+query, nil, fx.adminCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d, ждали 400: %s", query, rec.Code, rec.Body.String())
		}
	}

	// Доступ: без сессии — 401, teacher/student — 403.
	for _, tc := range []struct {
		name    string
		cookies []*http.Cookie
		want    int
	}{
		{"без сессии", nil, http.StatusUnauthorized},
		{"teacher", fx.teacherCookies, http.StatusForbidden},
		{"student", fx.annaCookies, http.StatusForbidden},
	} {
		rec := doRequest(t, fx.router, http.MethodGet, "/api/v1/admin/questions", nil, tc.cookies)
		if rec.Code != tc.want {
			t.Fatalf("%s: status=%d, ждали %d: %s", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
}

// TestAdminQuestionsDeleteOthers — admin удаляет чужие вопрос и ответ
// существующими маршрутами T71, посторонний teacher получает 403.
func TestAdminQuestionsDeleteOthers(t *testing.T) {
	fx := t85Seed(t)

	q1 := qaPostQuestion(t, fx.router, fx.course1ID, fx.lesson1ID,
		"Чужой вопрос для модерации", fx.annaCookies)
	q1ID, _ := q1["id"].(string)
	if q1ID == "" {
		t.Fatalf("q1 без id: %v", q1)
	}
	t85PostAnswer(t, fx.router, fx.course1ID, q1ID,
		"Ответ, который уйдёт каскадом.", fx.teacherCookies)

	q2 := qaPostQuestion(t, fx.router, fx.course2ID, fx.lesson3ID,
		"Вопрос про индексы для модерации", fx.borisCookies)
	q2ID, _ := q2["id"].(string)
	if q2ID == "" {
		t.Fatalf("q2 без id: %v", q2)
	}
	a2 := t85PostAnswer(t, fx.router, fx.course2ID, q2ID,
		"Ответ ученика, который удалит админ.", fx.borisCookies)
	a2ID, _ := a2["id"].(string)
	if a2ID == "" {
		t.Fatalf("a2 без id: %v", a2)
	}

	// Посторонний teacher не владеет курсом и не автор — 403.
	rec := doRequest(t, fx.router, http.MethodDelete,
		"/api/v1/courses/"+fx.course1ID+"/questions/"+q1ID, nil, fx.otherTeacherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher delete: status=%d, ждали 403: %s", rec.Code, rec.Body.String())
	}
	if _, ok := fx.app.Store.GetLessonQuestion(fx.course1ID, q1ID); !ok {
		t.Fatal("вопрос исчез после запрещённого удаления")
	}

	// Admin удаляет чужой вопрос — ответы уходят каскадом.
	rec = doRequest(t, fx.router, http.MethodDelete,
		"/api/v1/courses/"+fx.course1ID+"/questions/"+q1ID, nil, fx.adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete question: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := fx.app.Store.GetLessonQuestion(fx.course1ID, q1ID); ok {
		t.Fatal("вопрос не удалён админом")
	}
	after := t85Fetch(t, fx.router, fx.adminCookies, map[string]string{"courseId": fx.course1ID})
	if after.Total != 0 {
		t.Fatalf("после удаления вопроса в курсе осталось %d", after.Total)
	}

	// Admin удаляет чужой ответ.
	rec = doRequest(t, fx.router, http.MethodDelete,
		"/api/v1/courses/"+fx.course2ID+"/questions/"+q2ID+"/answers/"+a2ID, nil, fx.adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete answer: status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, ok := fx.app.Store.GetLessonQuestion(fx.course2ID, q2ID)
	if !ok {
		t.Fatal("вопрос q2 исчез после удаления ответа")
	}
	if len(got.Answers) != 0 {
		t.Fatalf("после удаления ответа осталось %d", len(got.Answers))
	}
}
