package contract_test

// Контрактные тесты T23 — «кабинет ученика»: честный прогресс по курсу
// (GET /me/courses/{id}/progress), профиль (GET/PATCH /me/profile) и история
// платежей (GET /me/payments). Идут сквозь NewRouter, как остальные
// контрактные тесты (T12).
//
// Особое внимание: completedLessonIds — всегда массив, никогда не null
// (фронтенд T30 пишется параллельно и уже наступал на null вместо []).

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// seedProgressCourse — опубликованный курс с двумя модулями и тремя
// video_text-уроками; порядок программы: м1-л1, м1-л2, м2-л1.
// Возвращает id курса и id уроков именно в этом порядке.
func seedProgressCourse(t *testing.T, app *handler.App, router http.Handler, priceCents int) (courseID string, lessonIDs []string) {
	t.Helper()

	adminCookies, _ := registerUser(t, router, "t23-admin@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "t23-admin@example.com"), repo.RoleAdmin) {
		t.Fatal("не удалось выдать роль admin")
	}
	teacherCookies, _ := registerUser(t, router, "t23-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "t23-teacher@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}

	var course map[string]any
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Курс прогресса", "description": "Для контрактных тестов T23", "priceCents": priceCents,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &course)
	courseID, _ = course["id"].(string)

	createModule := func(title string) string {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/modules", map[string]any{
			"title": title,
		}, teacherCookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create module: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var m map[string]any
		decodeBody(t, rec, &m)
		id, _ := m["id"].(string)
		return id
	}
	createLesson := func(moduleID, title string) string {
		rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
			"moduleId": moduleID, "title": title, "kind": "video_text", "bodyMd": "Текст урока",
		}, teacherCookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create lesson: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var l map[string]any
		decodeBody(t, rec, &l)
		id, _ := l["id"].(string)
		return id
	}

	m1 := createModule("Модуль 1")
	m2 := createModule("Модуль 2")
	lessonIDs = []string{
		createLesson(m1, "Урок 1"),
		createLesson(m1, "Урок 2"),
		createLesson(m2, "Урок 3"),
	}

	// Публикация: submit -> approve.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/submit", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodPost, "/api/v1/admin/courses/"+courseID+"/moderate", map[string]any{
		"decision": "approve",
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return courseID, lessonIDs
}

// enrollFree — запись на бесплатный курс активируется сразу, без webhook.
func enrollFree(t *testing.T, router http.Handler, courseID string, cookies []*http.Cookie) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// fetchProgress читает прогресс и заодно следит, что completedLessonIds —
// массив, а не null (сырой JSON: сервер обязан отдать []).
func fetchProgress(t *testing.T, router http.Handler, courseID string, cookies []*http.Cookie) (map[string]any, string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses/"+courseID+"/progress", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"completedLessonIds":null`) {
		t.Fatalf("completedLessonIds не должен быть null: %s", body)
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if _, ok := out["completedLessonIds"].([]any); !ok {
		t.Fatalf("completedLessonIds должен быть массивом: %s", body)
	}
	return out, body
}

// completedIDsOf — id пройденных уроков из ответа прогресса (строки, по порядку).
func completedIDsOf(t *testing.T, out map[string]any, body string) []string {
	t.Helper()
	raw, ok := out["completedLessonIds"].([]any)
	if !ok {
		t.Fatalf("completedLessonIds не массив (null?): %s", body)
	}
	ids := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("completedLessonIds содержит не строку: %v (%s)", v, body)
		}
		ids = append(ids, s)
	}
	return ids
}

// completeLesson проходит video_text-урок (тело можно не слать) и требует passed=true.
func completeLesson(t *testing.T, router http.Handler, courseID, lessonID string, cookies []*http.Cookie) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete %s: status=%d body=%s", lessonID, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["passed"] != true {
		t.Fatalf("урок %s не зачтён: %s", lessonID, rec.Body.String())
	}
}

// Пустой прогресс: completedLessonIds == [] (нигде не null), percent=0,
// nextLessonId — первый урок программы. Гарды: 401 без сессии, 403 без
// активной записи, 404 для несуществующего курса.
func TestMyCourseProgressEmpty(t *testing.T) {
	app, router := newTestApp()
	courseID, lessonIDs := seedProgressCourse(t, app, router, 0)

	studentCookies, _ := registerUser(t, router, "t23-student@example.com", "password123", "")

	// Без сессии — 401.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses/"+courseID+"/progress", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без входа: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Курс есть, но активной записи нет — 403.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/courses/"+courseID+"/progress", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("без записи: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Несуществующий курс — 404.
	rec = doRequest(t, router, http.MethodGet,
		"/api/v1/me/courses/00000000-0000-0000-0000-000000000000/progress", nil, studentCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("несуществующий курс: status=%d body=%s", rec.Code, rec.Body.String())
	}

	enrollFree(t, router, courseID, studentCookies)

	out, body := fetchProgress(t, router, courseID, studentCookies)
	if ids := completedIDsOf(t, out, body); len(ids) != 0 {
		t.Fatalf("прогресс должен быть пустым: %v (%s)", ids, body)
	}
	if out["courseId"] != courseID {
		t.Fatalf("courseId=%v, ждали %s: %s", out["courseId"], courseID, body)
	}
	if out["completedLessons"] != float64(0) || out["totalLessons"] != float64(3) {
		t.Fatalf("счётчики уроков: %s", body)
	}
	if out["percent"] != float64(0) {
		t.Fatalf("percent=%v, ждали 0: %s", out["percent"], body)
	}
	if out["completed"] != false {
		t.Fatalf("completed=%v, ждали false: %s", out["completed"], body)
	}
	if next, _ := out["nextLessonId"].(string); next != lessonIDs[0] {
		t.Fatalf("nextLessonId=%v, ждали первый урок %s: %s", out["nextLessonId"], lessonIDs[0], body)
	}
}

// Прохождение: урок попадает в список, percent растёт, nextLessonId сдвигается
// по программе (а не по времени прохождения); после всех уроков — completed.
func TestMyCourseProgressFlow(t *testing.T) {
	app, router := newTestApp()
	courseID, lessonIDs := seedProgressCourse(t, app, router, 0)

	studentCookies, _ := registerUser(t, router, "t23-flow@example.com", "password123", "")
	enrollFree(t, router, courseID, studentCookies)

	// Шаг 1: первый урок пройден.
	completeLesson(t, router, courseID, lessonIDs[0], studentCookies)
	out, body := fetchProgress(t, router, courseID, studentCookies)
	if got := completedIDsOf(t, out, body); !slices.Equal(got, lessonIDs[:1]) {
		t.Fatalf("после первого урока: %v (%s)", got, body)
	}
	if out["percent"] != float64(33) || out["completedLessons"] != float64(1) {
		t.Fatalf("percent/счётчики после первого урока: %s", body)
	}
	if next, _ := out["nextLessonId"].(string); next != lessonIDs[1] {
		t.Fatalf("nextLessonId=%v, ждали %s: %s", out["nextLessonId"], lessonIDs[1], body)
	}
	if out["completed"] != false {
		t.Fatalf("completed=%v, ждали false: %s", out["completed"], body)
	}

	// Шаг 2: пройден третий урок (второй пропущен). Список — в порядке
	// программы, а не в порядке прохождения; nextLessonId — первый
	// пропущенный по порядку, то есть второй.
	completeLesson(t, router, courseID, lessonIDs[2], studentCookies)
	out, body = fetchProgress(t, router, courseID, studentCookies)
	if got := completedIDsOf(t, out, body); !slices.Equal(got, []string{lessonIDs[0], lessonIDs[2]}) {
		t.Fatalf("порядок списка должен быть по программе: %v (%s)", got, body)
	}
	if out["percent"] != float64(66) {
		t.Fatalf("percent=%v, ждали 66: %s", out["percent"], body)
	}
	if next, _ := out["nextLessonId"].(string); next != lessonIDs[1] {
		t.Fatalf("nextLessonId=%v, ждали первый пропущенный %s: %s", out["nextLessonId"], lessonIDs[1], body)
	}

	// Шаг 3: пройдено всё — completed=true, nextLessonId=null.
	completeLesson(t, router, courseID, lessonIDs[1], studentCookies)
	out, body = fetchProgress(t, router, courseID, studentCookies)
	if got := completedIDsOf(t, out, body); !slices.Equal(got, lessonIDs) {
		t.Fatalf("всё пройдено: %v (%s)", got, body)
	}
	if out["percent"] != float64(100) || out["completedLessons"] != float64(3) {
		t.Fatalf("финальные счётчики: %s", body)
	}
	if out["completed"] != true {
		t.Fatalf("completed=%v, ждали true: %s", out["completed"], body)
	}
	if v, ok := out["nextLessonId"]; !ok || v != nil {
		t.Fatalf("nextLessonId должен быть null: %s", body)
	}
}

// Профиль: GET отдаёт пустое displayName (не null) и createdAt; PATCH
// валидирует длину 2..60, тримит пробелы, отклоняет лишние поля; GET
// подтверждает сохранение.
func TestMyProfileGetAndPatch(t *testing.T) {
	_, router := newTestApp()
	cookies, id := registerUser(t, router, "t23-profile@example.com", "password123", "")

	// GET до заполнения: displayName — пустая строка, не null.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/profile", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"displayName":null`) {
		t.Fatalf("displayName не должен быть null: %s", rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["id"] != id || out["email"] != "t23-profile@example.com" || out["role"] != "student" {
		t.Fatalf("профиль: %s", rec.Body.String())
	}
	if name, ok := out["displayName"].(string); !ok || name != "" {
		t.Fatalf("displayName до заполнения: %v (%s)", out["displayName"], rec.Body.String())
	}
	if created, ok := out["createdAt"].(string); !ok || created == "" {
		t.Fatalf("createdAt пуст: %s", rec.Body.String())
	}

	// Без входа — 401 на обоих методах.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/me/profile", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET без входа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", map[string]any{"displayName": "Иван"}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("PATCH без входа: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Невалидные тела: 1 символ, 61 символ, лишнее поле, пустое тело — 400.
	for _, bad := range []any{
		map[string]any{"displayName": "И"},
		map[string]any{"displayName": strings.Repeat("x", 61)},
		map[string]any{"displayName": "Иван", "лишнее": "поле"},
		map[string]any{},
	} {
		rec := doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", bad, cookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("PATCH %v: status=%d body=%s", bad, rec.Code, rec.Body.String())
		}
	}

	// 2 символа — ок; ответ — тот же объект профиля с новым именем.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", map[string]any{"displayName": "Ив"}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH 2 символа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["displayName"] != "Ив" || out["id"] != id {
		t.Fatalf("ответ PATCH: %s", rec.Body.String())
	}

	// Ровно 60 символов — ок (граница валидна).
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", map[string]any{
		"displayName": strings.Repeat("д", 60),
	}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH 60 символов: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Пробелы тримятся, GET подтверждает сохранение.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/me/profile", map[string]any{"displayName": "  Иван  "}, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH с пробелами: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["displayName"] != "Иван" {
		t.Fatalf("пробелы не тримятся: %s", rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/me/profile", nil, cookies)
	decodeBody(t, rec, &out)
	if out["displayName"] != "Иван" {
		t.Fatalf("имя не сохранилось: %s", rec.Body.String())
	}
}

// fetchPayments читает историю платежей и проверяет, что items — массив, не null.
func fetchPayments(t *testing.T, router http.Handler, path string, cookies []*http.Cookie) (map[string]any, string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, path, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("payments: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"items":null`) {
		t.Fatalf("items не должен быть null: %s", body)
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if _, ok := out["items"].([]any); !ok {
		t.Fatalf("items должен быть массивом: %s", body)
	}
	return out, body
}

// paymentRowsOf — строки истории платежей (каждая — объект).
func paymentRowsOf(t *testing.T, out map[string]any, body string) []map[string]any {
	t.Helper()
	raw, ok := out["items"].([]any)
	if !ok {
		t.Fatalf("items не массив (null?): %s", body)
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		row, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("строка платежа не объект: %v (%s)", v, body)
		}
		rows = append(rows, row)
	}
	return rows
}

// История платежей: пусто — items=[] (не null), total=0; после mock-покупки
// платёж виден сразу (pending, processedAt=null, courseTitle из курса),
// после webhook — succeeded с processedAt. Пагинация — конверт с page/limit.
func TestMyPayments(t *testing.T) {
	app, router := newTestApp()
	courseID, _ := seedProgressCourse(t, app, router, 10000)

	studentCookies, _ := registerUser(t, router, "t23-payments@example.com", "password123", "")

	// Пусто: items=[] (не null), total=0, дефолтная пагинация.
	out, body := fetchPayments(t, router, "/api/v1/me/payments", studentCookies)
	if rows := paymentRowsOf(t, out, body); len(rows) != 0 {
		t.Fatalf("пустая история: %v (%s)", rows, body)
	}
	if out["total"] != float64(0) || out["page"] != float64(1) || out["limit"] != float64(20) {
		t.Fatalf("конверт пагинации: %s", body)
	}

	// Покупка (mock): платёж pending — виден ученику сразу, processedAt пока null.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var enroll map[string]any
	decodeBody(t, rec, &enroll)
	pid, _ := enroll["providerPaymentId"].(string)
	if pid == "" {
		t.Fatalf("нет providerPaymentId: %s", rec.Body.String())
	}

	out, body = fetchPayments(t, router, "/api/v1/me/payments", studentCookies)
	rows := paymentRowsOf(t, out, body)
	if len(rows) != 1 {
		t.Fatalf("ожидали 1 платёж: %s", body)
	}
	row := rows[0]
	if row["status"] != "pending" || row["processedAt"] != nil {
		t.Fatalf("pending-платёж: %s", body)
	}
	if row["courseId"] != courseID || row["courseTitle"] != "Курс прогресса" {
		t.Fatalf("курс в строке платежа: %s", body)
	}
	if row["amountCents"] != float64(10000) || row["currency"] != "RUB" || row["provider"] != "mock" {
		t.Fatalf("деньги/провайдер: %s", body)
	}
	if created, _ := row["createdAt"].(string); created == "" {
		t.Fatalf("createdAt пуст: %s", body)
	}

	// Webhook успеха — платёж обработан, processedAt заполнен.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": pid, "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}

	out, body = fetchPayments(t, router, "/api/v1/me/payments", studentCookies)
	rows = paymentRowsOf(t, out, body)
	if len(rows) != 1 || rows[0]["status"] != "succeeded" {
		t.Fatalf("после webhook: %s", body)
	}
	if processed, ok := rows[0]["processedAt"].(string); !ok || processed == "" {
		t.Fatalf("processedAt после оплаты: %s", body)
	}

	// Пагинация: вторая страница при limit=1 — пустой массив, не null.
	out, body = fetchPayments(t, router, "/api/v1/me/payments?page=2&limit=1", studentCookies)
	if rows := paymentRowsOf(t, out, body); len(rows) != 0 {
		t.Fatalf("вторая страница должна быть пустой: %s", body)
	}
	if out["total"] != float64(1) || out["page"] != float64(2) || out["limit"] != float64(1) {
		t.Fatalf("конверт второй страницы: %s", body)
	}

	// Чужие платежи не видны.
	otherCookies, _ := registerUser(t, router, "t23-other@example.com", "password123", "")
	out, body = fetchPayments(t, router, "/api/v1/me/payments", otherCookies)
	if rows := paymentRowsOf(t, out, body); len(rows) != 0 {
		t.Fatalf("видны чужие платежи: %s", body)
	}
	if out["total"] != float64(0) {
		t.Fatalf("чужой total: %s", body)
	}
}
