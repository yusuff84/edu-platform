package contract_test

// Контрактные тесты T31 (P4): поиск по студенту и период в журнале платежей
// GET /admin/payments?q=&status=&courseId=&from=&to=&page=&limit=
// (Task/00-api-contract.md, раздел «Эндпоинты волны P4»):
//   - q — подстрока без учёта регистра по email ИЛИ displayName студента,
//     тримится, пустое значение фильтра не включает;
//   - from/to — ГГГГ-ММ-ДД (UTC), границы включительно, to — «до конца суток»;
//   - мусорная дата и from > to → 400;
//   - фильтры складываются по «И», пагинация — после фильтрации, а totals
//     считаются по тому же отфильтрованному набору, что и total (без пагинации).
// Postgres-путь проверяется только чтением кода: живого Postgres в прогоне нет
// (интеграционные тесты — T34).

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// t31Payment — платёж для журнала: сумма, статус и дата создания задаются явно.
type t31Payment struct {
	providerPaymentID string
	studentEmail      string
	amountCents       int
	status            repo.PaymentStatus
	createdAt         time.Time
}

// seedT31Payments создаёт платежи в in-memory store. CreatePayment всегда
// проставляет CreatedAt = time.Now(), поэтому дату для проверки периода
// переписываем у указателя, который хранит стор.
func seedT31Payments(t *testing.T, app *handler.App, courseID string, payments []t31Payment) {
	t.Helper()
	for _, pay := range payments {
		amount := pay.amountCents
		p, err := app.Store.CreatePayment(&repo.Payment{
			StudentID: userIDByEmail(t, app.Store, pay.studentEmail), CourseID: courseID,
			AmountCents: amount, PlatformFeeCents: amount * 15 / 100,
			TeacherAmountCents: amount - amount*15/100,
			Currency:           "RUB", Provider: "mock",
			ProviderPaymentID: pay.providerPaymentID, Status: pay.status,
		})
		if err != nil {
			t.Fatalf("создание платежа %s: %v", pay.providerPaymentID, err)
		}
		p.CreatedAt = pay.createdAt
	}
}

// fetchT31Journal вызывает GET /admin/payments с query и проверяет, что items —
// массив (не null), как требует контракт. Возвращает разобранный конверт,
// строки и сырое тело (для сообщений об ошибке).
func fetchT31Journal(t *testing.T, router http.Handler, cookies []*http.Cookie, query url.Values) (out map[string]any, items []any, body string) {
	t.Helper()
	path := "/api/v1/admin/payments"
	if len(query) > 0 {
		path += "?" + query.Encode()
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

// t31Row — строка items[i] как объект.
func t31Row(t *testing.T, items []any, i int, body string) map[string]any {
	t.Helper()
	row, ok := items[i].(map[string]any)
	if !ok {
		t.Fatalf("строка %d не объект (%v): %s", i, items[i], body)
	}
	return row
}

// t31Totals — секция totals конверта.
func t31Totals(t *testing.T, out map[string]any, body string) map[string]any {
	t.Helper()
	totals, ok := out["totals"].(map[string]any)
	if !ok {
		t.Fatalf("totals отсутствует или не объект: %s", body)
	}
	return totals
}

// t31Amount — сумма amountCents из секции totals.
func t31Amount(t *testing.T, out map[string]any, body string) float64 {
	t.Helper()
	return t31Totals(t, out, body)["amountCents"].(float64)
}

// t31WantErr требует код ответа и точный русский текст ошибки.
func t31WantErr(t *testing.T, rec *httptest.ResponseRecorder, code int, text string) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("status=%d, ждали %d (body=%s)", rec.Code, code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if out["error"] != text {
		t.Fatalf("error=%v, ждали %q (body=%s)", out["error"], text, rec.Body.String())
	}
}

// t31Query — query-строка журнала из пар «параметр → значение».
func t31Query(pairs map[string]string) url.Values {
	q := url.Values{}
	for k, v := range pairs {
		q.Set(k, v)
	}
	return q
}

// Поиск по студенту: часть email, часть displayName, другой регистр, пустое
// имя, триминг q, складывание с status/courseId, totals по фильтру и
// пагинация после фильтрации. Все платежи — сегодня (разное время суток),
// поэтому здесь же проверяются границы from/to на сегодня.
func TestAdminPaymentsJournalSearchByStudent(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Пять студентов: разные email и displayName; у carol имени нет —
	// пустое displayName не должно ломать поиск.
	for _, s := range []struct{ email, name string }{
		{"alice@example.com", "Алиса Иванова"},
		{"bob@example.com", "Борис Петров"},
		{"carol@example.com", ""},
		{"dave@example.com", "Дэйв Смит"},
		{"erin@example.com", "Эрин Ким"},
	} {
		registerUser(t, router, s.email, "password123", "")
		if s.name != "" && !app.Store.SetUserDisplayName(userIDByEmail(t, app.Store, s.email), s.name) {
			t.Fatalf("не удалось задать displayName для %s", s.email)
		}
	}

	// Дата «сегодня 12:00 UTC» — точка отсчёта, не зависящая от часового
	// пояса машины: платежи расставлены по часам этого дня (10:00..20:00).
	now := time.Now().UTC()
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	at := func(hour int) time.Time {
		return noon.Add(time.Duration(hour-12) * time.Hour)
	}
	day := func(offsetDays int) string {
		return noon.AddDate(0, 0, offsetDays).Format("2006-01-02")
	}
	seedT31Payments(t, app, courseID, []t31Payment{
		{"t31-p1", "alice@example.com", 10000, repo.PaymentSucceeded, at(10)},
		{"t31-p2", "alice@example.com", 20000, repo.PaymentPending, at(12)},
		{"t31-p3", "bob@example.com", 30000, repo.PaymentSucceeded, at(15)},
		{"t31-p4", "carol@example.com", 40000, repo.PaymentFailed, at(18)},
		{"t31-p5", "dave@example.com", 50000, repo.PaymentSucceeded, at(20)},
	})

	// Без фильтров — весь журнал: 150000 = сумма пяти платежей.
	out, items, body := fetchT31Journal(t, router, adminCookies, nil)
	wantNum(t, out, "total", 5)
	if len(items) != 5 {
		t.Fatalf("без фильтров: строк %d, ждали 5: %s", len(items), body)
	}
	totals := t31Totals(t, out, body)
	wantNum(t, totals, "amountCents", 150000)
	wantNum(t, totals, "platformFeeCents", 22500)
	wantNum(t, totals, "teacherAmountCents", 127500)

	// Поиск по части email: alice → только два её платежа, новые первыми,
	// totals — по этому же набору (20000 + 10000).
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": "alice"}))
	wantNum(t, out, "total", 2)
	if len(items) != 2 {
		t.Fatalf("q=alice: строк %d, ждали 2: %s", len(items), body)
	}
	if got := t31Row(t, items, 0, body)["providerPaymentId"]; got != "t31-p2" {
		t.Fatalf("q=alice: первая строка %v, ждали t31-p2 (новые первыми): %s", got, body)
	}
	if got := t31Row(t, items, 1, body)["providerPaymentId"]; got != "t31-p1" {
		t.Fatalf("q=alice: вторая строка %v, ждали t31-p1: %s", got, body)
	}
	if got := t31Row(t, items, 0, body)["studentEmail"]; got != "alice@example.com" {
		t.Fatalf("q=alice: studentEmail=%v: %s", got, body)
	}
	totals = t31Totals(t, out, body)
	wantNum(t, totals, "amountCents", 30000)
	wantNum(t, totals, "teacherAmountCents", 25500)

	// Тот же email в другом регистре.
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": "ALICE"}))
	wantNum(t, out, "total", 2)
	if len(items) != 2 {
		t.Fatalf("q=ALICE: строк %d, ждали 2: %s", len(items), body)
	}

	// Поиск по части displayName («Борис Петров» → 1 платёж).
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": "Петров"}))
	wantNum(t, out, "total", 1)
	if len(items) != 1 {
		t.Fatalf("q=Петров: строк %d, ждали 1: %s", len(items), body)
	}
	row := t31Row(t, items, 0, body)
	if row["studentName"] != "Борис Петров" || row["studentEmail"] != "bob@example.com" {
		t.Fatalf("q=Петров: строка не того студента: %s", body)
	}

	// displayName в нижнем регистре (кириллица): регистронезависимость.
	out, _, _ = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": "петров"}))
	wantNum(t, out, "total", 1)

	// Студент с пустым displayName находится по email, studentName — пустая строка.
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": "carol"}))
	wantNum(t, out, "total", 1)
	if len(items) != 1 || t31Row(t, items, 0, body)["studentName"] != "" {
		t.Fatalf("q=carol: ждали одну строку с пустым studentName: %s", body)
	}

	// q тримится; строка из пробелов = фильтра нет.
	out, _, _ = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": " alice "}))
	wantNum(t, out, "total", 2)
	out, _, _ = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"q": "   "}))
	wantNum(t, out, "total", 5)

	// q складывается со status по «И»: два платежа alice, статусы разные.
	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "status": "succeeded"}))
	wantNum(t, out, "total", 1)
	if len(items) != 1 || t31Row(t, items, 0, body)["providerPaymentId"] != "t31-p1" {
		t.Fatalf("q=alice&status=succeeded: ждали t31-p1: %s", body)
	}
	wantNum(t, t31Totals(t, out, body), "amountCents", 10000)

	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "status": "pending"}))
	wantNum(t, out, "total", 1)
	if len(items) != 1 || t31Row(t, items, 0, body)["providerPaymentId"] != "t31-p2" {
		t.Fatalf("q=alice&status=pending: ждали t31-p2: %s", body)
	}

	// q складывается с courseId: чужой курс — пусто, свой — оба платежа.
	out, _, _ = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "courseId": "00000000-0000-0000-0000-000000000000"}))
	wantNum(t, out, "total", 0)
	out, _, _ = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "courseId": courseID}))
	wantNum(t, out, "total", 2)

	// Период: сегодня и только сегодня (все платежи — сегодняшние),
	// границы дня включительны с обеих сторон.
	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"from": day(0), "to": day(0)}))
	wantNum(t, out, "total", 5)
	if len(items) != 5 {
		t.Fatalf("from=to=сегодня: строк %d, ждали 5: %s", len(items), body)
	}
	// from = завтра → пусто; to = вчера → пусто.
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"from": day(1)}))
	wantNum(t, out, "total", 0)
	if len(items) != 0 {
		t.Fatalf("from=завтра: строк %d, ждали 0: %s", len(items), body)
	}
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"to": day(-1)}))
	wantNum(t, out, "total", 0)
	if len(items) != 0 {
		t.Fatalf("to=вчера: строк %d, ждали 0: %s", len(items), body)
	}

	// Totals уменьшаются по фильтру: без фильтра 150000 → q=alice 30000
	// (проверено выше) → q+status 10000.
	unfiltered, _, unfilteredBody := fetchT31Journal(t, router, adminCookies, nil)
	filtered, _, filteredBody := fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "status": "succeeded"}))
	wantNum(t, t31Totals(t, filtered, filteredBody), "amountCents", 10000)
	if t31Amount(t, filtered, filteredBody) >= t31Amount(t, unfiltered, unfilteredBody) {
		t.Fatalf("totals по фильтру не меньше полных: %s", filteredBody)
	}

	// Пагинация — после фильтрации: q=alice даёт 2 записи, limit=1: страница 1
	// — t31-p2, страница 2 — t31-p1; total и totals не зависят от страницы.
	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "page": "2", "limit": "1"}))
	wantNum(t, out, "total", 2)
	wantNum(t, out, "page", 2)
	wantNum(t, out, "limit", 1)
	if len(items) != 1 || t31Row(t, items, 0, body)["providerPaymentId"] != "t31-p1" {
		t.Fatalf("q=alice&page=2&limit=1: ждали одну строку t31-p1: %s", body)
	}
	totals = t31Totals(t, out, body)
	wantNum(t, totals, "amountCents", 30000)
	wantNum(t, totals, "platformFeeCents", 4500)
	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"q": "alice", "page": "3", "limit": "1"}))
	wantNum(t, out, "total", 2)
	if len(items) != 0 {
		t.Fatalf("q=alice&page=3: строк %d, ждали 0: %s", len(items), body)
	}
}

// Период реально отсекает старые платежи: from исключает более ранние даты,
// to оставляет только до конца указанных суток, границы включительны,
// а total и totals считаются по отфильтрованному набору.
func TestAdminPaymentsJournalPeriod(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	for _, email := range []string{"period-a@example.com", "period-b@example.com"} {
		registerUser(t, router, email, "password123", "")
	}

	// Точка отсчёта — сегодня 12:00 UTC; платежи: сегодня, 3 дня назад,
	// 10 дней назад (даты строятся относительно time.Now().UTC()).
	now := time.Now().UTC()
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	day := func(offsetDays int) string {
		return noon.AddDate(0, 0, offsetDays).Format("2006-01-02")
	}
	seedT31Payments(t, app, courseID, []t31Payment{
		{"t31-r1", "period-a@example.com", 1000, repo.PaymentSucceeded, noon},
		{"t31-r2", "period-a@example.com", 2000, repo.PaymentSucceeded, noon.AddDate(0, 0, -3)},
		{"t31-r3", "period-b@example.com", 4000, repo.PaymentSucceeded, noon.AddDate(0, 0, -10)},
	})

	// Без фильтра — все три записи (1000 + 2000 + 4000).
	out, items, body := fetchT31Journal(t, router, adminCookies, nil)
	wantNum(t, out, "total", 3)
	if len(items) != 3 {
		t.Fatalf("без фильтра: строк %d, ждали 3: %s", len(items), body)
	}
	wantNum(t, t31Totals(t, out, body), "amountCents", 7000)

	// from = сегодня → только сегодняшний платёж.
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"from": day(0)}))
	wantNum(t, out, "total", 1)
	if len(items) != 1 || t31Row(t, items, 0, body)["providerPaymentId"] != "t31-r1" {
		t.Fatalf("from=сегодня: ждали t31-r1: %s", body)
	}
	wantNum(t, t31Totals(t, out, body), "amountCents", 1000)

	// from = 3 дня назад → сегодняшний и трёхдневной давности (граница
	// включительна), десятидневной давности — исключён.
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"from": day(-3)}))
	wantNum(t, out, "total", 2)
	if len(items) != 2 {
		t.Fatalf("from=3 дня назад: строк %d, ждали 2: %s", len(items), body)
	}
	wantNum(t, t31Totals(t, out, body), "amountCents", 3000)

	// to = 3 дня назад → трёх- и десятидневной давности (день границы целиком
	// включён), сегодняшний — исключён.
	out, items, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"to": day(-3)}))
	wantNum(t, out, "total", 2)
	if len(items) != 2 {
		t.Fatalf("to=3 дня назад: строк %d, ждали 2: %s", len(items), body)
	}
	wantNum(t, t31Totals(t, out, body), "amountCents", 6000)

	// Один и тот же день в from и to — границы включительны с обеих сторон.
	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"from": day(-3), "to": day(-3)}))
	wantNum(t, out, "total", 1)
	if len(items) != 1 || t31Row(t, items, 0, body)["providerPaymentId"] != "t31-r2" {
		t.Fatalf("from=to=3 дня назад: ждали t31-r2: %s", body)
	}
	wantNum(t, t31Totals(t, out, body), "amountCents", 2000)

	// В день без платежей — пусто, даже если период вокруг него.
	out, items, body = fetchT31Journal(t, router, adminCookies,
		t31Query(map[string]string{"from": day(-1), "to": day(-1)}))
	wantNum(t, out, "total", 0)
	if len(items) != 0 {
		t.Fatalf("from=to=вчера: строк %d, ждали 0: %s", len(items), body)
	}

	// to = сегодня → все записи (ничего не позже конца сегодняшних суток).
	out, _, body = fetchT31Journal(t, router, adminCookies, t31Query(map[string]string{"to": day(0)}))
	wantNum(t, out, "total", 3)
	wantNum(t, t31Totals(t, out, body), "amountCents", 7000)
}

// Валидация from/to: мусорные даты и from > to — 400 с русским текстом;
// равные from/to — не ошибка; журнал доступен только роли admin.
func TestAdminPaymentsJournalSearchValidation(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "admin@example.com")

	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?from=2026-13-45", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest, "Дата в формате ГГГГ-ММ-ДД")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?to=01-02-2026", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest, "Дата в формате ГГГГ-ММ-ДД")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?from=2026-10-20&to=2026-10-19", nil, adminCookies)
	t31WantErr(t, rec, http.StatusBadRequest, "Начало периода позже конца")

	// Одинаковые from и to — корректный однодневный период.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?from=2026-10-19&to=2026-10-19", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("from=to: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Только админ: студенту журнал недоступен.
	studentCookies, _ := registerUser(t, router, "not-admin@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?q=alice", nil, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("студент в журнале: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
