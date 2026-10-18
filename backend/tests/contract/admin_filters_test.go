package contract_test

// Контрактные тесты T48 (P6) — сортировка и фильтры админки:
//
//	GET  /admin/courses             — sort/dir (createdAt|updatedAt|title|
//	                                  price|students; вторичный ключ
//	                                  createdAt DESC), teacherEmail
//	                                  (подстрока без учёта регистра),
//	                                  from/to по createdAt (ГГГГ-ММ-ДД);
//	POST /admin/courses/{id}/unarchive — archived → draft, комментарий
//	                                  модерации сохраняется, 409/404/403;
//	GET  /admin/courses/review      — аддитивные teacherEmail/modulesCount/
//	                                  lessonsCount;
//	GET  /admin/enrollments         — q (email ИЛИ displayName ученика) и
//	                                  status (active|refunded|revoked).
//
// Формы ответов и тексты ошибок — по Task/00-api-contract.md, раздел
// «Эндпоинты волны P6». Здесь проверяется in-memory-реализация; SQL-ветка
// (postgres_admin_console.go) прогнана отдельно на живом Postgres — см.
// отчёт backend/tests/T48-admin-filters.md.

import (
	"net/http"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// ---- helpers T48 ----

// t48RegisterAdmin регистрирует пользователя и выдаёт ему роль admin.
func t48RegisterAdmin(t *testing.T, app *handler.App, router http.Handler, email string) []*http.Cookie {
	t.Helper()
	cookies, id := registerUser(t, router, email, "password123", "")
	if !app.Store.SetUserRole(id, repo.RoleAdmin) {
		t.Fatalf("не удалось выдать роль admin %s", email)
	}
	return cookies
}

// t48RegisterTeacher регистрирует преподавателя; возвращает cookie и id.
func t48RegisterTeacher(t *testing.T, app *handler.App, router http.Handler, email string) ([]*http.Cookie, string) {
	t.Helper()
	cookies, id := registerUser(t, router, email, "password123", "")
	if !app.Store.SetUserRole(id, repo.RoleTeacher) {
		t.Fatalf("не удалось выдать роль teacher %s", email)
	}
	return cookies, id
}

// t48CreateCourse создаёт черновик с заданными названием и ценой.
func t48CreateCourse(t *testing.T, router http.Handler, teacherCookies []*http.Cookie, title string, priceCents int) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": title, "description": "T48: описание тестового курса", "priceCents": priceCents,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("в ответе создания курса нет id: %s", rec.Body.String())
	}
	return id
}

// t48Publish публикует черновик напрямую в хранилище: POST /enrollments
// принимает только published-курс, а полный цикл модерации здесь не предмет
// проверки (он покрыт admin_console_test.go и admin_test.go).
func t48Publish(t *testing.T, app *handler.App, courseID string) {
	t.Helper()
	if !app.Store.SetCourseStatus(courseID, repo.CoursePublished, "") {
		t.Fatalf("не удалось опубликовать курс %s", courseID)
	}
}

// t48BuyActive оформляет ученику активную запись: бесплатный курс активируется
// сразу, платный — после подтверждающего вебхука mock (как на реальном пути).
// Возвращает providerPaymentId платежа — по нему тесты делают возврат средств.
func t48BuyActive(t *testing.T, router http.Handler, studentCookies []*http.Cookie, courseID string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	pid, _ := out["providerPaymentId"].(string)
	if pid == "" {
		t.Fatalf("в ответе покупки нет providerPaymentId: %s", rec.Body.String())
	}
	if active, _ := out["enrollmentActive"].(bool); active {
		return pid
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": pid, "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return pid
}

// t48FetchCourses вызывает GET /admin/courses с query и возвращает конверт,
// items (всегда массив) и сырое тело.
func t48FetchCourses(t *testing.T, router http.Handler, cookies []*http.Cookie, pairs map[string]string) (out map[string]any, items []any, body string) {
	t.Helper()
	path := "/api/v1/admin/courses"
	if len(pairs) > 0 {
		path += "?" + t31Query(pairs).Encode()
	}
	rec := doRequest(t, router, http.MethodGet, path, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	items, ok := out["items"].([]any)
	if !ok || items == nil {
		t.Fatalf("items должны быть массивом, не null: %s", rec.Body.String())
	}
	return out, items, rec.Body.String()
}

// t48CourseIDs — упорядоченный список id строк items.
func t48CourseIDs(t *testing.T, items []any, body string) []string {
	t.Helper()
	ids := make([]string, 0, len(items))
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("строка списка не объект (%v): %s", raw, body)
		}
		id, _ := row["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// t48WantOrder сравнивает фактический порядок id с ожидаемым.
func t48WantOrder(t *testing.T, got, want []string, what string) {
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

// ---- GET /admin/courses: сортировка и фильтры ----

// TestAdminCoursesSortingAndFilters: на четырёх курсах одного преподавателя
// проверяются asc/desc по title (регистронезависимо), price и students,
// вторичный ключ createdAt DESC (курсы C/D — полные близнецы), teacherEmail-
// фильтр, период from/to, пагинация после сортировки и 400 с текстами
// контракта на мусорные sort/dir/from.
func TestAdminCoursesSortingAndFilters(t *testing.T) {
	app, router := newTestApp()
	adminCookies := t48RegisterAdmin(t, app, router, "t48-sort-admin@example.com")
	teacherCookies, _ := t48RegisterTeacher(t, app, router, "t48-sort-teacher@example.com")
	otherTeacherCookies, _ := t48RegisterTeacher(t, app, router, "t48-other-teacher@example.com")

	// Чужой курс: не должен попадать в выборку по teacherEmail.
	otherCourseID := t48CreateCourse(t, router, otherTeacherCookies, "Чужой курс T48", 1)

	// A/B различаются по title/price, C и D — «близнецы» (одинаковые
	// title/price/students): по ним виден вторичный ключ createdAt DESC.
	// «альфа-сортировка» нарочно строчными: при сравнении без учёта регистра
	// она должна идти первой, при побайтовом сравнении — после «Бета…».
	// A и B публикуются сразу при создании: только на published-курс можно
	// оформить запись (POST /enrollments), и это же сохраняет порядок
	// updatedAt, который проверяется ниже (C/D созданы уже после публикаций).
	idA := t48CreateCourse(t, router, teacherCookies, "альфа-сортировка", 4900)
	t48Publish(t, app, idA)
	idB := t48CreateCourse(t, router, teacherCookies, "Бета-сортировка", 100)
	t48Publish(t, app, idB)
	idC := t48CreateCourse(t, router, teacherCookies, "Гамма-дубль", 9900)
	idD := t48CreateCourse(t, router, teacherCookies, "Гамма-дубль", 9900)

	// Ученики: у A — двое, у B — один (сортировка по students).
	student1, _ := registerUser(t, router, "t48-sort-student1@example.com", "password123", "")
	student2, _ := registerUser(t, router, "t48-sort-student2@example.com", "password123", "")
	student3, _ := registerUser(t, router, "t48-sort-student3@example.com", "password123", "")
	t48BuyActive(t, router, student1, idA)
	t48BuyActive(t, router, student2, idA)
	t48BuyActive(t, router, student3, idB)

	const teacherEmail = "t48-sort-teacher@example.com"
	filter := map[string]string{"teacherEmail": teacherEmail}

	// По умолчанию — createdAt DESC: старое поведение T25 не меняется.
	out, items, body := t48FetchCourses(t, router, adminCookies, filter)
	wantNum(t, out, "total", 4)
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idD, idC, idB, idA},
		"createdAt desc по умолчанию")

	// title: регистронезависимо; «Гамма-дубль» — вторичный ключ createdAt DESC.
	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "title", "dir": "asc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idA, idB, idD, idC}, "title asc")

	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "title", "dir": "desc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idD, idC, idB, idA}, "title desc")

	// price: 100 (B) < 4900 (A) < 9900 (C/D).
	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "price", "dir": "asc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idB, idA, idD, idC}, "price asc")

	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "price", "dir": "desc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idD, idC, idA, idB}, "price desc")

	// students: A — 2, B — 1, C/D — 0 (вторичный ключ createdAt DESC).
	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "students", "dir": "desc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idA, idB, idD, idC}, "students desc")

	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "students", "dir": "asc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idD, idC, idB, idA}, "students asc")

	// updatedAt: A/B тронуты публикацией вскоре после создания, C/D созданы
	// последними — порядок совпадает с createdAt desc.
	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "updatedAt", "dir": "desc"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idD, idC, idB, idA}, "updatedAt desc")

	// Пагинация применяется после сортировки: price asc → [B, A, D, C].
	_, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "sort": "price", "dir": "asc", "page": "2", "limit": "2"})
	t48WantOrder(t, t48CourseIDs(t, items, body), []string{idD, idC}, "price asc, страница 2")

	// teacherEmail — подстрока без учёта регистра; чужой курс отфильтрован.
	out, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{"teacherEmail": "T48-SORT-TEACHER"})
	wantNum(t, out, "total", 4)
	for _, raw := range items {
		if raw.(map[string]any)["teacherEmail"] != teacherEmail {
			t.Fatalf("teacherEmail-фильтр пропустил чужой курс: %s", body)
		}
	}
	for _, id := range t48CourseIDs(t, items, body) {
		if id == otherCourseID {
			t.Fatalf("teacherEmail-фильтр пропустил курс другого учителя: %s", body)
		}
	}
	// Нет совпадений — total 0, items [] (не null).
	out, items, body = t48FetchCourses(t, router, adminCookies, map[string]string{"teacherEmail": "нет-такого@example.test"})
	wantNum(t, out, "total", 0)
	if len(items) != 0 {
		t.Fatalf("items при пустом teacherEmail-фильтре: %s", body)
	}

	// from/to — включительно по createdAt (сегодня — курсы найдены).
	today := time.Now().UTC().Format("2006-01-02")
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	out, _, _ = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "from": today, "to": today})
	wantNum(t, out, "total", 4)
	out, _, _ = t48FetchCourses(t, router, adminCookies, map[string]string{
		"teacherEmail": teacherEmail, "from": tomorrow})
	wantNum(t, out, "total", 0)

	// Невалидные sort/dir и период — 400 с текстами контракта P6.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?sort=stars", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest,
		"Параметр sort должен быть одним из: createdAt, updatedAt, title, price, students")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?dir=sideways", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest, "Параметр dir должен быть asc или desc")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?from=2026-13-45", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest, "Дата в формате ГГГГ-ММ-ДД")
	rec = doRequest(t, router, http.MethodGet,
		"/api/v1/admin/courses?from="+tomorrow+"&to="+today, nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest, "Начало периода позже конца")
}

// ---- POST /admin/courses/{id}/unarchive ----

// TestAdminUnarchiveCourse: archived → draft с сохранением комментария
// модерации (курс отклонён с комментарием), повтор — 409, случайный/
// мусорный id — 404, студент — 403, аноним — 401.
func TestAdminUnarchiveCourse(t *testing.T) {
	app, router := newTestApp()
	adminCookies := t48RegisterAdmin(t, app, router, "t48-unarch-admin@example.com")
	teacherCookies, _ := t48RegisterTeacher(t, app, router, "t48-unarch-teacher@example.com")
	studentCookies, _ := registerUser(t, router, "t48-unarch-student@example.com", "password123", "")

	// Курс проходит модерацию с отказом: moderationComment заполнен и должен
	// пережить архивацию и возврат из архива.
	courseID := t48CreateCourse(t, router, teacherCookies, "Курс для unarchive T48", 1000)
	submitCourseThroughReview(t, router, teacherCookies, courseID)
	const comment = "T48: нужна доработка"
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/moderate", map[string]any{
		"decision": "reject", "comment": comment,
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// В архив (T25).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/archive", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if course, _ := out["course"].(map[string]any); course["status"] != "archived" {
		t.Fatalf("archive не перевёл курс в archived: %s", rec.Body.String())
	}

	// unarchive: archived → draft, комментарий модерации сохранён.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/unarchive", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	course, ok := out["course"].(map[string]any)
	if !ok {
		t.Fatalf("ответ unarchive без course: %s", rec.Body.String())
	}
	if course["id"] != courseID || course["status"] != "draft" {
		t.Fatalf("unarchive должен вернуть draft: %s", rec.Body.String())
	}
	if course["moderationComment"] != comment {
		t.Fatalf("комментарий модерации не сохранён (получили %v): %s", course["moderationComment"], rec.Body.String())
	}

	// Повторный unarchive (курс уже draft) — 409 с текстом контракта.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/unarchive", nil, adminCookies)
	t31WantErr(t, rec, http.StatusConflict, "Вернуть из архива можно только архивный курс")

	// Несуществующий курс — 404 (и для мусорного id тоже: «сущности нет»).
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "не-uuid"} {
		rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+id+"/unarchive", nil, adminCookies)
		t31WantErr(t, rec, http.StatusNotFound, "Курс не найден")
	}

	// Студент — 403 (маршрут под requireRole), аноним — 401.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/unarchive", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("студент на unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/unarchive", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- GET /admin/courses/review ----

// TestAdminReviewQueueEnriched: у курса на модерации в очереди появляются
// teacherEmail (email автора), modulesCount и lessonsCount — при сохранении
// прежних полей Course; после удаления автора teacherEmail — пустая строка.
func TestAdminReviewQueueEnriched(t *testing.T) {
	app, router := newTestApp()
	adminCookies := t48RegisterAdmin(t, app, router, "t48-queue-admin@example.com")
	teacherCookies, teacherID := t48RegisterTeacher(t, app, router, "t48-queue-teacher@example.com")

	// Курс с одним модулем и двумя уроками — уходит на модерацию.
	courseID := t48CreateCourse(t, router, teacherCookies, "Курс в очереди T48", 3000)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Модуль очереди",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var mod map[string]any
	decodeBody(t, rec, &mod)
	for i := 0; i < 2; i++ {
		rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
			"moduleId": mod["id"], "title": "Урок очереди", "kind": "video_text", "bodyMd": "текст",
		}, teacherCookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create lesson: status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/review", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("queue: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var queue map[string]any
	decodeBody(t, rec, &queue)
	items, _ := queue["items"].([]any)
	item := courseItemByID(t, items, courseID)

	// Старые поля Course не потеряны, новые — по контракту P6.
	for _, key := range []string{"id", "slug", "title", "teacherId", "status", "priceCents",
		"createdAt", "updatedAt", "teacherEmail", "modulesCount", "lessonsCount"} {
		if _, ok := item[key]; !ok {
			t.Fatalf("в очереди нет ключа %q: %s", key, rec.Body.String())
		}
	}
	if item["teacherEmail"] != "t48-queue-teacher@example.com" {
		t.Fatalf("teacherEmail очереди=%v: %s", item["teacherEmail"], rec.Body.String())
	}
	if item["status"] != "review" {
		t.Fatalf("статус очереди=%v: %s", item["status"], rec.Body.String())
	}
	wantNum(t, item, "modulesCount", 1)
	wantNum(t, item, "lessonsCount", 2)

	// Автор не найден: курс остаётся, teacherEmail — пустая строка.
	if err := app.Store.DeleteUser(teacherID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/review", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("queue после удаления автора: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &queue)
	items, _ = queue["items"].([]any)
	item = courseItemByID(t, items, courseID)
	if email, _ := item["teacherEmail"].(string); email != "" {
		t.Fatalf("teacherEmail без автора должен быть пустым, получили %q: %s", email, rec.Body.String())
	}
}

// ---- GET /admin/enrollments ----

// TestAdminEnrollmentsFilters: q ищет по email и по displayName (без учёта
// регистра), status фильтрует по active/refunded/revoked, фильтры
// складываются через AND, мусорный статус — 400 с текстом контракта.
func TestAdminEnrollmentsFilters(t *testing.T) {
	app, router := newTestApp()
	adminCookies := t48RegisterAdmin(t, app, router, "t48-enr-admin@example.com")
	teacherCookies, _ := t48RegisterTeacher(t, app, router, "t48-enr-teacher@example.com")

	// Бесплатный курс: запись активируется сразу после покупки.
	courseID := t48CreateCourse(t, router, teacherCookies, "Курс записей T48", 0)
	t48Publish(t, app, courseID)
	student1Cookies, student1ID := registerUser(t, router, "t48-enr-maria@example.com", "password123", "")
	student2Cookies, student2ID := registerUser(t, router, "t48-enr-second@example.com", "password123", "")
	student3Cookies, student3ID := registerUser(t, router, "t48-enr-third@example.com", "password123", "")
	// Имя ученика: q должен находить и по displayName (T23).
	rec := doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", map[string]any{
		"displayName": "Мария Тестова",
	}, student1Cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /me/profile: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Три активные записи; ниже вторая станет revoked, третья — refunded.
	t48BuyActive(t, router, student1Cookies, courseID)
	t48BuyActive(t, router, student2Cookies, courseID)
	thirdPID := t48BuyActive(t, router, student3Cookies, courseID)

	fetch := func(pairs map[string]string) (out map[string]any, items []any, body string) {
		t.Helper()
		path := "/api/v1/admin/enrollments"
		if len(pairs) > 0 {
			path += "?" + t31Query(pairs).Encode()
		}
		rec := doRequest(t, router, http.MethodGet, path, nil, adminCookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		decodeBody(t, rec, &out)
		items, ok := out["items"].([]any)
		if !ok || items == nil {
			t.Fatalf("items должны быть массивом, не null: %s", rec.Body.String())
		}
		return out, items, rec.Body.String()
	}

	// q по email (подстрока в другом регистре).
	out, _, _ := fetch(map[string]string{"q": "T48-ENR-MARIA"})
	wantNum(t, out, "total", 1)

	// q по displayName.
	out, items, body := fetch(map[string]string{"q": "мария"})
	wantNum(t, out, "total", 1)
	row := items[0].(map[string]any)
	if row["studentId"] != student1ID || row["studentEmail"] != "t48-enr-maria@example.com" {
		t.Fatalf("q по имени нашёл чужую запись: %s", body)
	}

	// Нет совпадений — total 0, items [] (не null).
	out, items, body = fetch(map[string]string{"q": "нет-такого"})
	wantNum(t, out, "total", 0)
	if len(items) != 0 {
		t.Fatalf("items при пустом q: %s", body)
	}

	// status=active — все три записи; чужой статус — пусто.
	out, _, _ = fetch(map[string]string{"status": "active"})
	wantNum(t, out, "total", 3)
	out, _, _ = fetch(map[string]string{"status": "refunded"})
	wantNum(t, out, "total", 0)

	// Отзыв доступа (T16): вторая запись становится revoked.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/admin/enrollments/"+student2ID+"/courses/"+courseID+"/revoke", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: status=%d body=%s", rec.Code, rec.Body.String())
	}
	out, items, body = fetch(map[string]string{"status": "revoked"})
	wantNum(t, out, "total", 1)
	if items[0].(map[string]any)["studentId"] != student2ID {
		t.Fatalf("status=revoked вернул чужую запись: %s", body)
	}
	out, _, _ = fetch(map[string]string{"status": "active"})
	wantNum(t, out, "total", 2)

	// Возврат средств (T16): третья запись становится refunded.
	rec = webhookMock(t, router, thirdPID, "refunded")
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook refunded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	out, items, body = fetch(map[string]string{"status": "refunded"})
	wantNum(t, out, "total", 1)
	if items[0].(map[string]any)["studentId"] != student3ID {
		t.Fatalf("status=refunded вернул чужую запись: %s", body)
	}
	out, _, _ = fetch(map[string]string{"status": "active"})
	wantNum(t, out, "total", 1)

	// q и status складываются через AND.
	out, _, _ = fetch(map[string]string{"q": "second", "status": "revoked"})
	wantNum(t, out, "total", 1)
	out, _, _ = fetch(map[string]string{"q": "second", "status": "active"})
	wantNum(t, out, "total", 0)

	// Невалидный статус — 400 с текстом контракта P6.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments?status=expired", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest,
		"Некорректный статус записи: допустимы active, refunded, revoked")
}
