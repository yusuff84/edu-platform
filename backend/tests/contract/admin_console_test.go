package contract_test

// Контрактные тесты T25 — админ-консоль супер-админа:
//   GET  /admin/stats                          — сводка состояния платформы;
//   GET  /admin/courses                        — курсы всех статусов;
//   POST /admin/courses/{id}/archive|unpublish — смена статуса;
//   GET  /admin/courses/{id}/reviews           — отзывы для модерации;
//   DELETE /admin/reviews/{id}                 — удаление отзыва;
//   GET  /admin/enrollments                    — все записи на курсы.
//
// Форма ответов — ровно по Task/T25-admin-console.md (фронтенд T27
// пишется параллельно): items всегда массив, суммы в копейках. Отдельно
// закреплено, что новые маршруты не затирают существующую очередь
// модерации /admin/courses/review и её контракт.

import (
	"net/http"
	"net/url"
	"testing"

	"edu-platform/backend/internal/repo"
)

// adminStats запрашивает /admin/stats и возвращает разобранный ответ.
func adminStats(t *testing.T, router http.Handler, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/stats", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// statsSection достаёт вложенный объект сводки и падает, если его нет.
func statsSection(t *testing.T, body map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := body[key].(map[string]any)
	if !ok {
		t.Fatalf("секция %q отсутствует или не объект: %v", key, body)
	}
	return v
}

// wantNum проверяет числовое поле (JSON-числа декодируются во float64).
func wantNum(t *testing.T, obj map[string]any, key string, want float64) {
	t.Helper()
	got, ok := obj[key].(float64)
	if !ok {
		t.Fatalf("поле %q отсутствует или не число: %v (объект: %v)", key, obj[key], obj)
	}
	if got != want {
		t.Fatalf("%s=%v, ждали %v (объект: %v)", key, got, want, obj)
	}
}

// wantKeys проверяет, что объект содержит ровно перечисленные ключи
// (контракт T25: лишних полей фронт не ждёт, недостающих не должно быть).
func wantKeys(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			t.Fatalf("нет ключа %q (объект: %v)", k, obj)
		}
	}
	if len(obj) != len(keys) {
		t.Fatalf("в объекте %d ключей, ждали %d: %v", len(obj), len(keys), obj)
	}
}

// courseItemByID находит строку списка курсов по id.
func courseItemByID(t *testing.T, items []any, id string) map[string]any {
	t.Helper()
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("курс %s не найден среди %d строк", id, len(items))
	return nil
}

// enrollRowByEmail находит строку записи на курс по email ученика.
func enrollRowByEmail(t *testing.T, items []any, email string) map[string]any {
	t.Helper()
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if row["studentEmail"] == email {
			return row
		}
	}
	t.Fatalf("запись ученика %s не найдена среди %d строк", email, len(items))
	return nil
}

// createDraftCourse создаёт черновик от имени учителя и возвращает его id.
func createDraftCourse(t *testing.T, router http.Handler, teacherCookies []*http.Cookie, title string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": title, "description": "Описание для теста T25", "priceCents": 0,
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

// submitCourseThroughReview прогоняет черновик через модуль, урок и submit.
func submitCourseThroughReview(t *testing.T, router http.Handler, teacherCookies []*http.Cookie, courseID string) {
	t.Helper()
	var mod map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
		"title": "Модуль T25",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &mod)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mod["id"], "title": "Урок T25", "kind": "video_text", "bodyMd": "текст",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Сценарий 1: форма /stats и нули на пустой базе ----

// TestAdminConsoleStatsEmptyAndShape: агрегаты пустого хранилища равны нулю
// (Store напрямую), а форма ответа содержит все ключи контракта; ни одна
// секция не приходит null/пустой.
func TestAdminConsoleStatsEmptyAndShape(t *testing.T) {
	app, router := newTestApp()

	// Пустое хранилище: сводка — нулевая структура без исключений.
	if zero := app.Store.AdminOverview(); zero != (repo.AdminTotals{}) {
		t.Fatalf("на пустом хранилище сводка не нулевая: %+v", zero)
	}

	// Через API: админ нужен для доступа (значит users.total/admins/newLast7 = 1).
	adminCookies, adminID := registerUser(t, router, "stats-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "stats-admin@example.com")
	if adminID == "" {
		t.Fatal("registerUser не вернул id админа")
	}

	body := adminStats(t, router, adminCookies)

	users := statsSection(t, body, "users")
	wantKeys(t, users, "total", "students", "teachers", "admins", "blocked", "newLast7")
	wantNum(t, users, "total", 1)
	wantNum(t, users, "students", 0)
	wantNum(t, users, "teachers", 0)
	wantNum(t, users, "admins", 1)
	wantNum(t, users, "blocked", 0)
	wantNum(t, users, "newLast7", 1)

	courses := statsSection(t, body, "courses")
	wantKeys(t, courses, "total", "draft", "review", "published", "archived")
	for k := range courses {
		wantNum(t, courses, k, 0)
	}

	payments := statsSection(t, body, "payments")
	wantKeys(t, payments, "count", "succeededCents", "platformFeeCents", "teacherCents", "last30Cents")
	for k := range payments {
		wantNum(t, payments, k, 0)
	}

	enrollments := statsSection(t, body, "enrollments")
	wantKeys(t, enrollments, "total", "active")
	wantNum(t, enrollments, "total", 0)
	wantNum(t, enrollments, "active", 0)

	reviews := statsSection(t, body, "reviews")
	wantKeys(t, reviews, "total", "avgRating")
	wantNum(t, reviews, "total", 0)
	wantNum(t, reviews, "avgRating", 0)
}

// ---- Сценарий 2: /stats после успешной оплаты ----

// TestAdminConsoleStatsAfterPayment: успешная оплата увеличивает count и
// succeededCents; суммы сплита берутся из полей платежа; 30-дневное окно
// включает свежий платёж.
func TestAdminConsoleStatsAfterPayment(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// До оплаты: платежей (и записей) нет.
	before := statsSection(t, adminStats(t, router, adminCookies), "payments")
	wantNum(t, before, "count", 0)
	wantNum(t, before, "succeededCents", 0)

	// Покупка + подтверждение webhook-ом (mock).
	studentCookies, _ := registerUser(t, router, "stats-student@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var enroll map[string]any
	decodeBody(t, rec, &enroll)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": enroll["providerPaymentId"], "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}

	body := adminStats(t, router, adminCookies)

	payments := statsSection(t, body, "payments")
	wantNum(t, payments, "count", 1)
	wantNum(t, payments, "succeededCents", 10000)
	wantNum(t, payments, "platformFeeCents", 1500)
	wantNum(t, payments, "teacherCents", 8500)
	wantNum(t, payments, "last30Cents", 10000)

	enrollments := statsSection(t, body, "enrollments")
	wantNum(t, enrollments, "total", 1)
	wantNum(t, enrollments, "active", 1)

	courses := statsSection(t, body, "courses")
	wantNum(t, courses, "total", 1)
	wantNum(t, courses, "published", 1)

	users := statsSection(t, body, "users")
	wantNum(t, users, "total", 3) // админ, учитель, ученик
	wantNum(t, users, "students", 1)
	wantNum(t, users, "teachers", 1)
	wantNum(t, users, "admins", 1)
	wantNum(t, users, "newLast7", 3)
}

// ---- Сценарий 3: GET /admin/courses ----

// TestAdminConsoleCoursesList: админу видны курсы всех статусов; фильтры
// status/q, пагинация и форма строки контракта.
func TestAdminConsoleCoursesList(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, publishedID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Три новых курса: draft, review, archived.
	draftID := createDraftCourse(t, router, teacherCookies, "Черновик админки")
	reviewID := createDraftCourse(t, router, teacherCookies, "Курс на модерации админки")
	submitCourseThroughReview(t, router, teacherCookies, reviewID)
	archivedID := createDraftCourse(t, router, teacherCookies, "Архивный курс админки")
	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+archivedID+"/archive", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Все 4 статуса видны админу.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("courses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 4)
	wantNum(t, list, "page", 1)
	wantNum(t, list, "limit", 20)
	items, _ := list["items"].([]any)
	if len(items) != 4 {
		t.Fatalf("items=%d, ждали 4: %s", len(items), rec.Body.String())
	}
	statuses := map[string]bool{}
	for _, raw := range items {
		statuses[raw.(map[string]any)["status"].(string)] = true
	}
	for _, st := range []string{"draft", "review", "published", "archived"} {
		if !statuses[st] {
			t.Fatalf("статус %s не виден админу: %s", st, rec.Body.String())
		}
	}

	// Форма строки — ровно как в контракте, счётчики и email учителя.
	// T54 (волна P7): в строку аддитивно добавлен coverPath (string|null).
	published := courseItemByID(t, items, publishedID)
	wantKeys(t, published, "id", "slug", "title", "coverPath", "teacherId", "teacherEmail",
		"status", "priceCents", "createdAt", "updatedAt", "modulesCount", "lessonsCount",
		"studentsCount")
	if v, ok := published["coverPath"]; !ok || v != nil {
		t.Fatalf("у курса без обложки coverPath=%v (ok=%v), ждали null", v, ok)
	}
	if published["teacherEmail"] != "teacher@example.com" {
		t.Fatalf("teacherEmail=%v: %s", published["teacherEmail"], rec.Body.String())
	}
	wantNum(t, published, "modulesCount", 1) // У seedPublishedCourse 1 модуль
	wantNum(t, published, "lessonsCount", 1)
	wantNum(t, published, "studentsCount", 0)
	wantNum(t, published, "priceCents", 10000)

	// Фильтр status=published.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?status=published", nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 1)
	items, _ = list["items"].([]any)
	if items[0].(map[string]any)["id"] != publishedID {
		t.Fatalf("status=published вернул чужой курс: %s", rec.Body.String())
	}

	// Фильтр status=review.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?status=review", nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 1)
	items, _ = list["items"].([]any)
	if items[0].(map[string]any)["id"] != reviewID {
		t.Fatalf("status=review вернул чужой курс: %s", rec.Body.String())
	}

	// Статус вне списка — 400.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?status="+url.QueryEscape("мусор"), nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=мусор: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Поиск q — подстрока по названию, регистронезависимо (кириллица).
	rec = doRequest(t, router, http.MethodGet,
		"/api/v1/admin/courses?q="+url.QueryEscape("ЧЕРНОВИК"), nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 1)
	items, _ = list["items"].([]any)
	if items[0].(map[string]any)["id"] != draftID {
		t.Fatalf("q не нашёл черновик: %s", rec.Body.String())
	}

	// Пустой результат — items пустой массив, не null.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?q="+url.QueryEscape("нетакого"), nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 0)
	if empty, ok := list["items"].([]any); !ok || len(empty) != 0 {
		t.Fatalf("items при пустом результате должен быть []: %s", rec.Body.String())
	}

	// Пагинация: новые первыми; page=2&limit=2 — вторая половина.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?page=2&limit=2", nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 4)
	wantNum(t, list, "page", 2)
	wantNum(t, list, "limit", 2)
	items, _ = list["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("страница 2 при limit=2: items=%d: %s", len(items), rec.Body.String())
	}
	// Порядок: published создан первым (старейший), значит на второй странице последним.
	if items[1].(map[string]any)["id"] != publishedID {
		t.Fatalf("сортировка «новые первыми» нарушена: %s", rec.Body.String())
	}

	// limit > 100 обрезается.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses?limit=500", nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "limit", 100)

	// Тень маршрута: GET /admin/courses/review — по-прежнему очередь
	// модерации (свой обработчик), а не список T25.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/review", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("review queue: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var queue map[string]any
	decodeBody(t, rec, &queue)
	wantNum(t, queue, "total", 1)
	if _, hasPage := queue["page"]; hasPage {
		t.Fatalf("review queue отвечает лишним page — затёрт списком T25: %s", rec.Body.String())
	}
	qItems, _ := queue["items"].([]any)
	if len(qItems) != 1 || qItems[0].(map[string]any)["id"] != reviewID {
		t.Fatalf("очередь модерации сломана: %s", rec.Body.String())
	}
	// T48 добавил очереди teacherEmail/modulesCount/lessonsCount (аддитивно,
	// по контракту P6), поэтому «тень маршрута» проверяем по полю списка T25,
	// которого у очереди нет и быть не должно: studentsCount.
	if _, hasStudents := qItems[0].(map[string]any)["studentsCount"]; hasStudents {
		t.Fatalf("review queue отвечает полем списка T25 — маршрут затёрт: %s", rec.Body.String())
	}

	// И она по-прежнему чиста от status-фильтров T25 (мусор игнорируется
	// старым обработчиком — он не читает query вообще).
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/review?status=мусор", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("review queue со status: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Сценарий 4: archive ----

// TestAdminConsoleArchiveCourse: published → archived; повторно — 409;
// чужой/несуществующий id — 404.
func TestAdminConsoleArchiveCourse(t *testing.T) {
	app, router := newTestApp()
	_, publishedID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+publishedID+"/archive", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	course, ok := out["course"].(map[string]any)
	if !ok {
		t.Fatalf("ответ archive без course: %s", rec.Body.String())
	}
	if course["id"] != publishedID || course["status"] != "archived" {
		t.Fatalf("форма ответа archive нарушена: %s", rec.Body.String())
	}

	// Повторная архивация — 409 (конфликт состояния).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+publishedID+"/archive", nil, adminCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("повторный archive: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Архивный курс исчез из публичного каталога.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses", nil, nil)
	var catalog map[string]any
	decodeBody(t, rec, &catalog)
	wantNum(t, catalog, "total", 0)

	// Несуществующий курс — 404 (и для мусорного id тоже).
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "не-uuid"} {
		rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+id+"/archive", nil, adminCookies)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("archive %q: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
	}
}

// ---- Сценарий 5: unpublish ----

// TestAdminConsoleUnpublishCourse: published → draft; для draft — 409;
// несуществующий курс — 404.
func TestAdminConsoleUnpublishCourse(t *testing.T) {
	app, router := newTestApp()
	_, publishedID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+publishedID+"/unpublish", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("unpublish: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	course, ok := out["course"].(map[string]any)
	if !ok {
		t.Fatalf("ответ unpublish без course: %s", rec.Body.String())
	}
	if course["id"] != publishedID || course["status"] != "draft" {
		t.Fatalf("unpublish должен вернуть draft: %s", rec.Body.String())
	}

	// Курс пропал из каталога.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses", nil, nil)
	var catalog map[string]any
	decodeBody(t, rec, &catalog)
	wantNum(t, catalog, "total", 0)

	// Повторный unpublish (теперь draft) — 409.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+publishedID+"/unpublish", nil, adminCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("unpublish черновика: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий курс — 404.
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/admin/courses/00000000-0000-0000-0000-000000000000/unpublish", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unpublish чужого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Сценарий 6: отзывы ----

// TestAdminConsoleReviewsModeration: список отзывов курса с email, удаление
// с пересчётом рейтинга курса и сводки, повторное удаление — 404.
func TestAdminConsoleReviewsModeration(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный: запись активна сразу
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Два ученика с отзывами 5 и 4 → среднее 4.5.
	studentACookies, studentAID := registerUser(t, router, "review-a@example.com", "password123", "")
	studentBCookies, studentBID := registerUser(t, router, "review-b@example.com", "password123", "")
	for _, c := range [][]*http.Cookie{studentACookies, studentBCookies} {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
			"courseId": courseID, "provider": "mock",
		}, c)
		if rec.Code != http.StatusCreated {
			t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	sendReview := func(cookies []*http.Cookie, rating int, body string) string {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
			"rating": rating, "body": body,
		}, cookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("review: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		decodeBody(t, rec, &out)
		id, _ := out["id"].(string)
		return id
	}
	reviewAID := sendReview(studentACookies, 5, "Отличный курс")
	reviewBID := sendReview(studentBCookies, 4, "Хороший курс")

	// Список отзывов курса в админке.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID+"/reviews", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin reviews: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	items, _ := list["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("отзывов %d, ждали 2: %s", len(items), rec.Body.String())
	}
	foundA, foundB := false, false
	for _, raw := range items {
		row := raw.(map[string]any)
		// T69: teacherReply/teacherReplyAt добавлены аддитивно во все виды
		// отзывов (контракт волны P8), поэтому ключей теперь 8.
		wantKeys(t, row, "id", "studentId", "studentEmail", "rating", "body", "createdAt",
			"teacherReply", "teacherReplyAt")
		if row["teacherReply"] != "" || row["teacherReplyAt"] != nil {
			t.Fatalf("без ответа teacherReply=%v teacherReplyAt=%v: ждали пусто/null",
				row["teacherReply"], row["teacherReplyAt"])
		}
		switch row["studentId"] {
		case studentAID:
			foundA = true
			if row["id"] != reviewAID || row["studentEmail"] != "review-a@example.com" ||
				row["rating"] != float64(5) || row["body"] != "Отличный курс" {
				t.Fatalf("форма отзыва A нарушена: %s", rec.Body.String())
			}
		case studentBID:
			foundB = true
			if row["id"] != reviewBID || row["studentEmail"] != "review-b@example.com" || row["rating"] != float64(4) {
				t.Fatalf("форма отзыва B нарушена: %s", rec.Body.String())
			}
		}
	}
	if !foundA || !foundB {
		t.Fatalf("в списке нет отзывов обоих учеников: %s", rec.Body.String())
	}

	// Сводка: 2 отзыва, среднее 4.5.
	stats := adminStats(t, router, adminCookies)
	reviews := statsSection(t, stats, "reviews")
	wantNum(t, reviews, "total", 2)
	wantNum(t, reviews, "avgRating", 4.5)

	// Удаление отзыва A — 204 без тела.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/reviews/"+reviewAID, nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete review: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 должен быть без тела: %q", rec.Body.String())
	}

	// Повторное удаление — 404.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/reviews/"+reviewAID, nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("повторное удаление: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Несуществующий отзыв — 404.
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/admin/reviews/00000000-0000-0000-0000-000000000000", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужой отзыв: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Отзыв исчез, остался только B.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID+"/reviews", nil, adminCookies)
	decodeBody(t, rec, &list)
	items, _ = list["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != reviewBID {
		t.Fatalf("после удаления список неверен: %s", rec.Body.String())
	}

	// Рейтинг курса пересчитан (RecalcCourseRating): был 4.5, стал 4.
	c, ok := app.Store.GetCourseByID(courseID)
	if !ok {
		t.Fatal("курс пропал")
	}
	if c.AvgRating != 4 || c.RatingsCount != 1 {
		t.Fatalf("рейтинг курса не пересчитан: avg=%v count=%d", c.AvgRating, c.RatingsCount)
	}
	stats = adminStats(t, router, adminCookies)
	reviews = statsSection(t, stats, "reviews")
	wantNum(t, reviews, "total", 1)
	wantNum(t, reviews, "avgRating", 4)

	// Несуществующий курс — 404.
	rec = doRequest(t, router, http.MethodGet,
		"/api/v1/admin/courses/00000000-0000-0000-0000-000000000000/reviews", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("reviews чужого курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Пустой список отзывов — items [], не null.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/reviews/"+reviewBID, nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete review B: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses/"+courseID+"/reviews", nil, adminCookies)
	decodeBody(t, rec, &list)
	if empty, ok := list["items"].([]any); !ok || len(empty) != 0 {
		t.Fatalf("items без отзывов должен быть []: %s", rec.Body.String())
	}
}

// ---- Сценарий 7: записи на курсы ----

// TestAdminConsoleEnrollments: список записей с данными ученика/курса и
// процентом прогресса; фильтры courseId/studentId комбинируются через AND;
// пагинация и сортировка «новые первыми»; 400 на мусорные UUID.
func TestAdminConsoleEnrollments(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Второй урок: 1 из 2 пройденных = 50%.
	mods := app.Store.ListModulesByCourse(courseID)
	if len(mods) != 1 {
		t.Fatalf("модулей %d, ждали 1", len(mods))
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": mods[0].ID, "title": "Урок два", "kind": "video_text", "bodyMd": "текст",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson 2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)

	studentCookies, studentID := registerUser(t, router, "enroll-row@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lesson["id"].(string)+"/complete", nil, studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Список: одна запись, полная форма контракта.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("enrollments: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 1)
	wantNum(t, list, "page", 1)
	wantNum(t, list, "limit", 20)
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%d, ждали 1: %s", len(items), rec.Body.String())
	}
	row := items[0].(map[string]any)
	wantKeys(t, row, "id", "studentId", "studentEmail", "studentName", "courseId",
		"courseTitle", "status", "createdAt", "completedLessons", "totalLessons", "percent")
	if row["studentId"] != studentID || row["studentEmail"] != "enroll-row@example.com" {
		t.Fatalf("данные ученика неверны: %s", rec.Body.String())
	}
	if row["studentName"] != "" {
		t.Fatalf("имя без display_name должно быть пустым: %s", rec.Body.String())
	}
	if row["courseId"] != courseID || row["courseTitle"] != "Go для новичков" {
		t.Fatalf("данные курса неверны: %s", rec.Body.String())
	}
	if row["status"] != "active" {
		t.Fatalf("статус записи=%v: %s", row["status"], rec.Body.String())
	}
	wantNum(t, row, "completedLessons", 1)
	wantNum(t, row, "totalLessons", 2)
	wantNum(t, row, "percent", 50)

	// Фильтр по courseId.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments?courseId="+courseID, nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 1)

	// Фильтр по studentId.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments?studentId="+studentID, nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 1)

	// Комбинация AND: чужой courseId + наш studentId → пусто (items []).
	rec = doRequest(t, router, http.MethodGet,
		"/api/v1/admin/enrollments?courseId=00000000-0000-0000-0000-000000000000&studentId="+studentID,
		nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 0)
	if empty, ok := list["items"].([]any); !ok || len(empty) != 0 {
		t.Fatalf("items при пустом фильтре должен быть []: %s", rec.Body.String())
	}

	// Второй ученик: новые первыми, пагинация.
	student2Cookies, student2ID := registerUser(t, router, "enroll-row2@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, student2Cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll 2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = student2ID

	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments?page=1&limit=1", nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 2)
	items, _ = list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("page=1&limit=1: items=%d: %s", len(items), rec.Body.String())
	}
	if items[0].(map[string]any)["studentEmail"] != "enroll-row2@example.com" {
		t.Fatalf("сортировка «новые первыми» нарушена: %s", rec.Body.String())
	}

	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments?page=2&limit=1", nil, adminCookies)
	decodeBody(t, rec, &list)
	wantNum(t, list, "total", 2)
	items, _ = list["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["studentEmail"] != "enroll-row@example.com" {
		t.Fatalf("вторая страница неверна: %s", rec.Body.String())
	}

	// Мусорные параметры — 400.
	for _, q := range []string{
		"courseId=" + url.QueryEscape("не-uuid"),
		"studentId=" + url.QueryEscape("не-uuid"),
		"limit=abc",
	} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/enrollments?"+q, nil, adminCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d body=%s", q, rec.Code, rec.Body.String())
		}
	}
}

// ---- Сценарий 8: не-админ на всех эндпоинтах ----

// TestAdminConsoleRoleGuards: студент и учитель получают 403 на каждом
// эндпоинте T25, аноним — 401.
func TestAdminConsoleRoleGuards(t *testing.T) {
	app, router := newTestApp()
	studentCookies, _ := registerUser(t, router, "console-student@example.com", "password123", "")
	teacherCookies, _ := registerUser(t, router, "console-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "console-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	const someID = "00000000-0000-0000-0000-000000000000"
	checks := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/admin/stats"},
		{http.MethodGet, "/api/v1/admin/courses"},
		{http.MethodPost, "/api/v1/admin/courses/" + someID + "/archive"},
		{http.MethodPost, "/api/v1/admin/courses/" + someID + "/unpublish"},
		{http.MethodGet, "/api/v1/admin/courses/" + someID + "/reviews"},
		{http.MethodDelete, "/api/v1/admin/reviews/" + someID},
		{http.MethodGet, "/api/v1/admin/enrollments"},
	}
	for _, c := range checks {
		for _, who := range []struct {
			name    string
			cookies []*http.Cookie
		}{
			{"студент", studentCookies},
			{"учитель", teacherCookies},
		} {
			rec := doRequest(t, router, c.method, c.path, nil, who.cookies)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s на %s %s: status=%d body=%s", who.name, c.method, c.path, rec.Code, rec.Body.String())
			}
		}
	}
	// Без сессии — 401 (requireAuth срабатывает раньше requireRole).
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/stats", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним на /admin/stats: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
