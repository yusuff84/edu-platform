package contract_test

// Контрактные тесты T22 — ученики курса и статистика для учителя:
// GET /teacher/courses/{id}/students и GET /teacher/courses/{id}/stats.
// Форма ответов — Task/00-api-contract.md (T22); фронтенд T29 пишется
// параллельно и ждёт ровно эти имена полей: studentId, email, displayName,
// enrolledAt, status, completedLessons, totalLessons, percent, lastActivityAt.
// items — всегда массив (не null), lastActivityAt — null у не начинавшего.

import (
	"net/http"
	"testing"
	"time"

	"edu-platform/backend/internal/repo"
)

// t22VideoLesson добавляет video_text-урок в модуль курса и возвращает его id.
// Курс может быть опубликован: TeacherCreateLesson статус не проверяет —
// так же поступает progress_test.go.
func t22VideoLesson(t *testing.T, router http.Handler, teacherCookies []*http.Cookie, courseID, moduleID, title string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", map[string]any{
		"moduleId": moduleID, "title": title, "kind": "video_text", "bodyMd": "текст урока",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lesson %q: status=%d body=%s", title, rec.Code, rec.Body.String())
	}
	var lesson map[string]any
	decodeBody(t, rec, &lesson)
	id, _ := lesson["id"].(string)
	return id
}

// t22Enroll оформляет покупку курса через публичный API (201) и возвращает
// ответ (у платного курса providerPaymentId нужен для webhook).
func t22Enroll(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID string) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// t22Webhook шлёт mock-webhook со статусом и проверяет 200.
func t22Webhook(t *testing.T, router http.Handler, providerPaymentID, status string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": providerPaymentID, "status": status,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook %s: status=%d body=%s", status, rec.Code, rec.Body.String())
	}
}

// t22Students запрашивает список учеников: отдаёт разобранный конверт при
// ожидаемом статусе 200 (иначе — t.Fatalf).
func t22Students(t *testing.T, router http.Handler, cookies []*http.Cookie, courseID, query string) map[string]any {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/students"+query, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("students%s: status=%d body=%s", query, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	return out
}

// t22StudentRow проверяет точный набор полей строки ученика (контракт T29):
// ни лишних, ни пропущенных.
func t22StudentRow(t *testing.T, row map[string]any) map[string]any {
	t.Helper()
	want := []string{"studentId", "email", "displayName", "enrolledAt", "status",
		"completedLessons", "totalLessons", "percent", "lastActivityAt"}
	if len(row) != len(want) {
		t.Fatalf("в строке ученика %d полей, ждали %d: %v", len(row), len(want), row)
	}
	for _, k := range want {
		if _, ok := row[k]; !ok {
			t.Fatalf("нет поля %q в строке ученика: %v", k, row)
		}
	}
	return row
}

// Пустой курс: items — пустой массив (не null), total=0, дефолтная пагинация;
// статистика пустого курса — нули. Доступ: несуществующий курс — 404,
// чужой — 403, ученик — 403, нечисловые page/limit — 400.
func TestTeacherStudentsEmptyAndAccess(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	// Пустой курс.
	out := t22Students(t, router, teacherCookies, courseID, "")
	items, ok := out["items"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("items пустого курса — пустой массив, не null: %v", out["items"])
	}
	if total, _ := out["total"].(float64); total != 0 {
		t.Fatalf("total=%v, ждали 0: %v", out["total"], out)
	}
	if page, _ := out["page"].(float64); page != 1 {
		t.Fatalf("page=%v, ждали 1: %v", out["page"], out)
	}
	if limit, _ := out["limit"].(float64); limit != 20 {
		t.Fatalf("limit=%v, ждали 20: %v", out["limit"], out)
	}

	// Статистика пустого курса: программа есть (1 модуль, 1 урок из сида),
	// учеников и отзывов нет — счётчики нулевые.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/stats", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stats map[string]any
	decodeBody(t, rec, &stats)
	for field, want := range map[string]float64{
		"studentsTotal": 0, "studentsActive": 0, "studentsCompleted": 0,
		"avgPercent": 0, "revenueCents": 0, "modulesTotal": 1, "lessonsTotal": 1,
		"reviewsCount": 0, "avgRating": 0,
	} {
		if got, _ := stats[field].(float64); got != want {
			t.Fatalf("stats[%s]=%v, ждали %v: %s", field, stats[field], want, rec.Body.String())
		}
	}

	// Несуществующий курс — 404.
	missing := "00000000-0000-0000-0000-000000000000"
	for _, path := range []string{"/students", "/stats"} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+missing+path, nil, teacherCookies)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s несуществующего курса: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	// Чужой учитель — 403.
	otherCookies, _ := registerUser(t, router, "other-teacher-t22@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "other-teacher-t22@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	for _, path := range []string{"/students", "/stats"} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+path, nil, otherCookies)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s чужого курса: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	// Ученик без роли teacher — 403 (requireRole родительского роутера).
	studentCookies, _ := registerUser(t, router, "plain-t22@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/students", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("студент в students: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Нечисловые page/limit — 400.
	for _, q := range []string{"?limit=abc", "?page=xyz"} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/students"+q, nil, teacherCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("students%s: status=%d, ждали 400: %s", q, rec.Code, rec.Body.String())
		}
	}
}

// Два ученика, часть уроков пройдена: completedLessons/percent верны,
// у непроходившего lastActivityAt == null, сортировка — новые записи первыми.
func TestTeacherStudentsProgressAndOrder(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0) // бесплатный
	mods := app.Store.ListModulesByCourse(courseID)
	if len(mods) == 0 {
		t.Fatal("в курсе нет модулей")
	}
	// Сид даёт 1 quiz-урок; добавляем 2 видео → всего 3 урока.
	videoA := t22VideoLesson(t, router, teacherCookies, courseID, mods[0].ID, "Видео 1")
	_ = t22VideoLesson(t, router, teacherCookies, courseID, mods[0].ID, "Видео 2")

	annaCookies, annaID := registerUser(t, router, "anna-t22@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(annaID, "Анна Иванова") {
		t.Fatal("не удалось задать имя Анны")
	}
	borisCookies, borisID := registerUser(t, router, "boris-t22@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(borisID, "Борис Петров") {
		t.Fatal("не удалось задать имя Бориса")
	}

	t22Enroll(t, router, annaCookies, courseID)
	t22Enroll(t, router, borisCookies, courseID)

	// Анна проходит один видеоурок; Борис не начинал.
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/me/courses/"+courseID+"/lessons/"+videoA+"/complete", map[string]any{}, annaCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: status=%d body=%s", rec.Code, rec.Body.String())
	}

	out := t22Students(t, router, teacherCookies, courseID, "")
	if total, _ := out["total"].(float64); total != 2 {
		t.Fatalf("total=%v, ждали 2: %v", out["total"], out)
	}
	items, _ := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items=%d, ждали 2: %v", len(items), out)
	}
	// Борис записан позже — он первый (сортировка enrolledAt DESC).
	first := t22StudentRow(t, items[0].(map[string]any))
	second := t22StudentRow(t, items[1].(map[string]any))
	if first["studentId"] != borisID || second["studentId"] != annaID {
		t.Fatalf("порядок неверный: первый %v, второй %v", first["studentId"], second["studentId"])
	}

	// Борис: 0 из 3, percent 0, lastActivityAt — null (ключ присутствует).
	if first["email"] != "boris-t22@example.com" || first["displayName"] != "Борис Петров" {
		t.Fatalf("данные Бориса: %v", first)
	}
	if first["status"] != "active" {
		t.Fatalf("статус Бориса=%v", first["status"])
	}
	if first["completedLessons"] != float64(0) || first["totalLessons"] != float64(3) || first["percent"] != float64(0) {
		t.Fatalf("прогресс Бориса: %v", first)
	}
	if v := first["lastActivityAt"]; v != nil {
		t.Fatalf("lastActivityAt непроходившего должен быть null, got %v", v)
	}

	// Анна: 1 из 3 → 33 (округление вниз); lastActivityAt — валидный RFC3339.
	if second["completedLessons"] != float64(1) || second["totalLessons"] != float64(3) || second["percent"] != float64(33) {
		t.Fatalf("прогресс Анны: %v", second)
	}
	last, ok := second["lastActivityAt"].(string)
	if !ok || last == "" {
		t.Fatalf("lastActivityAt Анны должен быть строкой: %v", second["lastActivityAt"])
	}
	if _, err := time.Parse(time.RFC3339, last); err != nil {
		t.Fatalf("lastActivityAt не RFC3339: %q (%v)", last, err)
	}
	if enrolled, _ := second["enrolledAt"].(string); enrolled == "" {
		t.Fatalf("enrolledAt пуст: %v", second)
	} else if _, err := time.Parse(time.RFC3339, enrolled); err != nil {
		t.Fatalf("enrolledAt не RFC3339: %q (%v)", enrolled, err)
	}
}

// Поиск q (по email и имени, регистронезависимо, с TrimSpace) и пагинация:
// limit=1 даёт 1 элемент при total=2, страницы не теряют записи.
func TestTeacherStudentsSearchAndPagination(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 0)

	annaCookies, annaID := registerUser(t, router, "anna-search-t22@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(annaID, "Анна Иванова") {
		t.Fatal("не удалось задать имя Анны")
	}
	borisCookies, borisID := registerUser(t, router, "boris-search-t22@example.com", "password123", "")
	if !app.Store.SetUserDisplayName(borisID, "Борис Петров") {
		t.Fatal("не удалось задать имя Бориса")
	}
	t22Enroll(t, router, annaCookies, courseID)
	t22Enroll(t, router, borisCookies, courseID)

	// fetch: total + элементы, с проверкой «items — массив, не null».
	fetch := func(query string) (int, []any) {
		t.Helper()
		out := t22Students(t, router, teacherCookies, courseID, query)
		raw, ok := out["items"]
		if !ok || raw == nil {
			t.Fatalf("students%s: items отсутствует или null: %v", query, out)
		}
		items, _ := raw.([]any)
		total, _ := out["total"].(float64)
		return int(total), items
	}
	firstID := func(items []any) string {
		row := items[0].(map[string]any)
		id, _ := row["studentId"].(string)
		return id
	}

	// По email — регистронезависимо.
	total, items := fetch("?q=ANNA-SEARCH")
	if total != 1 || len(items) != 1 || firstID(items) != annaID {
		t.Fatalf("q=ANNA-SEARCH: total=%d items=%v", total, items)
	}
	// По имени — регистронезависимо; пробелы вокруг q обрезаются (TrimSpace).
	total, items = fetch("?q=%20аНнА%20")
	if total != 1 || len(items) != 1 || firstID(items) != annaID {
		t.Fatalf("q=« аНнА »: total=%d items=%v", total, items)
	}
	total, items = fetch("?q=петров")
	if total != 1 || len(items) != 1 || firstID(items) != borisID {
		t.Fatalf("q=петров: total=%d items=%v", total, items)
	}
	// Подстрока по общему домену — оба ученика.
	total, items = fetch("?q=search-t22@example.com")
	if total != 2 || len(items) != 2 {
		t.Fatalf("q=домен: total=%d items=%v", total, items)
	}
	// Нет совпадений — total=0 и пустой массив.
	total, items = fetch("?q=нет-такого-ученика")
	if total != 0 || len(items) != 0 {
		t.Fatalf("q=нет совпадений: total=%d items=%v", total, items)
	}

	// Пагинация: limit=1 — один элемент, total остаётся 2; первым идёт Борис
	// (новая запись), на второй странице — Анна.
	total, items = fetch("?limit=1")
	if total != 2 || len(items) != 1 || firstID(items) != borisID {
		t.Fatalf("limit=1: total=%d items=%v", total, items)
	}
	total, items = fetch("?page=2&limit=1")
	if total != 2 || len(items) != 1 || firstID(items) != annaID {
		t.Fatalf("page=2&limit=1: total=%d items=%v", total, items)
	}

	// limit > 100 обрезается до 100; page=0/limit=0 — дефолты 1/20.
	out := t22Students(t, router, teacherCookies, courseID, "?limit=500")
	if limit, _ := out["limit"].(float64); limit != 100 {
		t.Fatalf("limit=500: limit=%v, ждали 100: %v", out["limit"], out)
	}
	out = t22Students(t, router, teacherCookies, courseID, "?page=0&limit=0")
	if page, _ := out["page"].(float64); page != 1 {
		t.Fatalf("page=0: page=%v, ждали 1: %v", out["page"], out)
	}
	if limit, _ := out["limit"].(float64); limit != 20 {
		t.Fatalf("limit=0: limit=%v, ждали 20: %v", out["limit"], out)
	}
}

// Статистика курса: totals/active/completed/avgPercent, выручка только по
// успешным платежам (pending и refunded не считаются), модули/уроки, отзывы.
func TestTeacherCourseStats(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 10000) // платный
	mods := app.Store.ListModulesByCourse(courseID)
	if len(mods) == 0 {
		t.Fatal("в курсе нет модулей")
	}
	// Сид: 1 quiz-урок, добавляем 2 видео → всего 3 урока.
	_ = t22VideoLesson(t, router, teacherCookies, courseID, mods[0].ID, "Видео 1")
	_ = t22VideoLesson(t, router, teacherCookies, courseID, mods[0].ID, "Видео 2")
	lessons := app.Store.ListLessonsByCourse(courseID)
	if len(lessons) != 3 {
		t.Fatalf("уроков=%d, ждали 3", len(lessons))
	}
	quizLesson, videoA, videoB := lessons[0].ID, lessons[1].ID, lessons[2].ID

	annaCookies, annaID := registerUser(t, router, "stats-anna-t22@example.com", "password123", "")
	borisCookies, borisID := registerUser(t, router, "stats-boris-t22@example.com", "password123", "")
	dmitryCookies, dmitryID := registerUser(t, router, "stats-dmitry-t22@example.com", "password123", "")
	veraCookies, _ := registerUser(t, router, "stats-vera-t22@example.com", "password123", "")

	// Анна и Борис оплачивают курс, Дмитрий оплачивает и получает возврат,
	// Вера бросает покупку на pending — такая оплата в выручку не попадает.
	t22Webhook(t, router, t22Enroll(t, router, annaCookies, courseID)["providerPaymentId"].(string), "succeeded")
	t22Webhook(t, router, t22Enroll(t, router, borisCookies, courseID)["providerPaymentId"].(string), "succeeded")
	dmitryPID := t22Enroll(t, router, dmitryCookies, courseID)["providerPaymentId"].(string)
	t22Webhook(t, router, dmitryPID, "succeeded")
	t22Webhook(t, router, dmitryPID, "refunded")
	_ = t22Enroll(t, router, veraCookies, courseID) // pending, без webhook

	// Прохождение: Анна — все 3 урока, Борис — 2 видео.
	complete := func(cookies []*http.Cookie, lessonID string, body map[string]any) {
		t.Helper()
		rec := doRequest(t, router, http.MethodPost,
			"/api/v1/me/courses/"+courseID+"/lessons/"+lessonID+"/complete", body, cookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("complete %s: status=%d body=%s", lessonID, rec.Code, rec.Body.String())
		}
	}
	complete(annaCookies, quizLesson, map[string]any{"quizAnswers": map[string]int{"q1": 0}})
	complete(annaCookies, videoA, map[string]any{})
	complete(annaCookies, videoB, map[string]any{})
	complete(borisCookies, videoA, map[string]any{})
	complete(borisCookies, videoB, map[string]any{})

	// Сводка: 3 записи, 2 активные, 1 прошёл курс целиком,
	// avgPercent = (100 + 66 + 0) / 3 = 55 (округление вниз),
	// выручка = 10000 + 10000: pending Веры и refunded Дмитрия не считаются.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/stats", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stats map[string]any
	decodeBody(t, rec, &stats)
	for field, want := range map[string]float64{
		"studentsTotal": 3, "studentsActive": 2, "studentsCompleted": 1,
		"avgPercent": 55, "revenueCents": 20000, "modulesTotal": 1,
		"lessonsTotal": 3, "reviewsCount": 0, "avgRating": 0,
	} {
		if got, _ := stats[field].(float64); got != want {
			t.Fatalf("stats[%s]=%v, ждали %v: %s", field, stats[field], want, rec.Body.String())
		}
	}

	// Список включает отчисленного Дмитрия (все статусы) и отражает прогресс:
	// Анна 3/3 = 100, Борис 2/3 = 66; у не начинавшего Дмитрия lastActivityAt
	// null, Вера (pending) в ученики не попадает — записи на курс у неё нет.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/students", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("students: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list map[string]any
	decodeBody(t, rec, &list)
	rowsByID := make(map[string]map[string]any)
	for _, raw := range list["items"].([]any) {
		row := t22StudentRow(t, raw.(map[string]any))
		rowsByID[row["studentId"].(string)] = row
	}
	if len(rowsByID) != 3 {
		t.Fatalf("в списке %d учеников, ждали 3: %s", len(rowsByID), rec.Body.String())
	}
	annaRow := rowsByID[annaID]
	if annaRow == nil || annaRow["percent"] != float64(100) || annaRow["completedLessons"] != float64(3) {
		t.Fatalf("строка Анны: %v", annaRow)
	}
	if annaRow["lastActivityAt"] == nil {
		t.Fatalf("lastActivityAt Анны не должен быть null: %v", annaRow)
	}
	borisRow := rowsByID[borisID]
	if borisRow == nil || borisRow["percent"] != float64(66) || borisRow["completedLessons"] != float64(2) {
		t.Fatalf("строка Бориса: %v", borisRow)
	}
	dmitryRow := rowsByID[dmitryID]
	if dmitryRow == nil || dmitryRow["status"] != "refunded" {
		t.Fatalf("строка отчисленного Дмитрия: %v", dmitryRow)
	}
	if dmitryRow["lastActivityAt"] != nil {
		t.Fatalf("lastActivityAt Дмитрия должен быть null: %v", dmitryRow)
	}
	if _, exists := rowsByID[userIDByEmail(t, app.Store, "stats-vera-t22@example.com")]; exists {
		t.Fatal("Вера (pending-платёж) не должна попадать в список учеников")
	}

	// Отзывы: Анна 5, Борис 4 → reviewsCount=2, avgRating=4.5.
	for i, c := range []struct {
		cookies []*http.Cookie
		rating  int
	}{{annaCookies, 5}, {borisCookies, 4}} {
		rec = doRequest(t, router, http.MethodPost, "/api/v1/courses/"+courseID+"/reviews", map[string]any{
			"rating": c.rating, "body": "Отзыв",
		}, c.cookies)
		if rec.Code != http.StatusCreated {
			t.Fatalf("review %d: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/stats", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats после отзывов: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &stats)
	if stats["reviewsCount"] != float64(2) || stats["avgRating"] != float64(4.5) {
		t.Fatalf("отзывы в статистике: %s", rec.Body.String())
	}
}
