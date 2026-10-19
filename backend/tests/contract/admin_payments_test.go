package contract_test

// Контрактные тесты US6 (журнал платежей и комиссия) — эндпоинты от T07:
// GET /admin/payments, GET/PUT /admin/settings. Подтверждают форму ответа
// Task/00-api-contract.md: в журнале есть email/имя студента (расхождение №6
// закрыто T16 — раньше здесь был тест-фиксатор, ожидавший ПОЛНОГО ОТСУТСТВИЯ
// этих полей).

import (
	"net/http"
	"testing"
)

// Журнал платежей: пуст на старте (items — пустой массив, не null),
// после покупки — 1 запись, фильтры, пагинация, totals.
func TestAdminPaymentsJournal(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Пустой журнал на старте: items присутствует и не null.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/payments", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("journal: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var journal map[string]any
	decodeBody(t, rec, &journal)
	if items, ok := journal["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("items должен быть пустым массивом, а не null/отсутствовать: %s", rec.Body.String())
	}
	if total, _ := journal["total"].(float64); total != 0 {
		t.Fatalf("total=%v на старте", journal["total"])
	}

	// Покупка за другого студента.
	studentCookies, _ := registerUser(t, router, "journal-student@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
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

	// Журнал: 1 запись, форма контракта US6, totals = сумма строк выборки.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 1 {
		t.Fatalf("total=%v body=%s", journal["total"], rec.Body.String())
	}
	items, _ := journal["items"].([]any)
	row := items[0].(map[string]any)
	if row["amountCents"] != float64(10000) || row["platformFeeCents"] != float64(1500) || row["teacherAmountCents"] != float64(8500) {
		t.Fatalf("сплит в журнале неверный: %s", rec.Body.String())
	}
	if row["courseTitle"] != "Go для новичков" || row["provider"] != "mock" || row["status"] != "succeeded" {
		t.Fatalf("форма строки журнала нарушена: %s", rec.Body.String())
	}
	if row["processedAt"] == nil {
		t.Fatalf("processedAt должен быть заполнен после webhook: %s", rec.Body.String())
	}
	studentID, _ := row["studentId"].(string)
	if studentID == "" {
		t.Fatalf("нет studentId: %s", rec.Body.String())
	}
	// T16 (п.3): email и имя студента в журнале — расхождение №6 закрыто.
	// Раньше здесь стоял тест-фиксатор, падавший при появлении studentEmail.
	if row["studentEmail"] != "journal-student@example.com" {
		t.Fatalf("studentEmail отсутствует или неверен: %s", rec.Body.String())
	}
	if name, hasName := row["studentName"]; !hasName {
		t.Fatalf("в строке журнала нет studentName: %s", rec.Body.String())
	} else if _, isString := name.(string); !isString {
		t.Fatalf("studentName должен быть строкой (допустимо пустой), got %v", name)
	}
	if row["studentId"] == row["studentEmail"] {
		t.Fatalf("studentEmail не должен дублировать UUID studentId: %s", rec.Body.String())
	}
	totals, _ := journal["totals"].(map[string]any)
	if totals["amountCents"] != float64(10000) || totals["platformFeeCents"] != float64(1500) || totals["teacherAmountCents"] != float64(8500) {
		t.Fatalf("totals не равны сумме строк: %s", rec.Body.String())
	}

	// Фильтр по status.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?status=pending", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 0 {
		t.Fatalf("status=pending: total=%v", journal["total"])
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?status=succeeded", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 1 {
		t.Fatalf("status=succeeded: total=%v", journal["total"])
	}
	// Некорректный статус — 400.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?status=magic", nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("некорректный status: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Фильтр по courseId.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?courseId="+courseID, nil, adminCookies)
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 1 {
		t.Fatalf("courseId=%s: total=%v", courseID, journal["total"])
	}
	rec = doRequest(t, router, http.MethodGet,
		"/api/v1/admin/payments?courseId=00000000-0000-0000-0000-000000000000", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 0 {
		t.Fatalf("чужой courseId: total=%v", journal["total"])
	}
	// courseId не-UUID — 400.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?courseId=не-uuid", nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("courseId не UUID: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// total не зависит от page; limit > 100 обрезается до 100; page=0 → первая.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?page=2&limit=1", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if total, _ := journal["total"].(float64); total != 1 {
		t.Fatalf("total на второй странице=%v (должен быть прежним)", journal["total"])
	}
	if page, _ := journal["page"].(float64); page != 2 {
		t.Fatalf("page=%v, ждали 2", journal["page"])
	}
	if pageItems, _ := journal["items"].([]any); len(pageItems) != 0 {
		t.Fatalf("на странице 2 при 1 записи items должны быть пустыми: %s", rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?limit=500", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if limit, _ := journal["limit"].(float64); limit != 100 {
		t.Fatalf("limit=%v, ждали clamp до 100", journal["limit"])
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?page=0", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if page, _ := journal["page"].(float64); page != 1 {
		t.Fatalf("page=0 должен стать 1, got %v", journal["page"])
	}
	// Нечисловой limit — 400.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?limit=abc", nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit=abc: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Клампинг limit (фактическое поведение handler):
	//   <=0 → 20 (по умолчанию), 1 → 1 (минимум НЕ округляется до 20),
	//   >100 → 100. Закрепляем факт, чтобы изменение было заметным.
	for _, c := range []struct {
		query string
		limit float64
	}{
		{"limit=0", 20},
		{"limit=-5", 20},
		{"limit=1", 1},
		{"limit=100", 100},
		{"limit=101", 100},
	} {
		rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?"+c.query, nil, adminCookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", c.query, rec.Code, rec.Body.String())
		}
		decodeBody(t, rec, &journal)
		if limit, _ := journal["limit"].(float64); limit != c.limit {
			t.Fatalf("%s: limit=%v, ждали %v", c.query, journal["limit"], c.limit)
		}
	}

	// Второй платёж: totals считаются по ВСЕМ записям с учётом фильтров,
	// но БЕЗ пагинации — на любой странице totals суммируют всё.
	student2, _ := registerUser(t, router, "journal-student2@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, student2)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll 2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &enroll)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": enroll["providerPaymentId"], "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook 2: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?page=2&limit=1", nil, adminCookies)
	decodeBody(t, rec, &journal)
	if pageItems, _ := journal["items"].([]any); len(pageItems) != 1 {
		t.Fatalf("страница 2 при limit=1 должна содержать 1 запись: %s", rec.Body.String())
	}
	totals, _ = journal["totals"].(map[string]any)
	if totals["amountCents"] != float64(20000) || totals["platformFeeCents"] != float64(3000) || totals["teacherAmountCents"] != float64(17000) {
		t.Fatalf("totals на странице 2 должны суммировать ВСЕ записи: %s", rec.Body.String())
	}
	if total, _ := journal["total"].(float64); total != 2 {
		t.Fatalf("total=%v, ждали 2", journal["total"])
	}
	// Строка на странице 2 (старейший платёж) тоже несёт email студента
	// (не UUID). Сортировка журнала — новые первыми, значит page 2 = первый
	// платёж journal-student.
	decodeBody(t, rec, &journal)
	items, _ = journal["items"].([]any)
	row2 := items[0].(map[string]any)
	if row2["studentEmail"] != "journal-student@example.com" {
		t.Fatalf("studentEmail второй строки: %s", rec.Body.String())
	}
}

// Настройки комиссии: GET возвращает текущую, PUT меняет,
// новое значение применяется к НОВОЙ покупке; валидация 0..90.
func TestAdminSettingsCommission(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Текущая комиссия.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/settings", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var settings map[string]any
	decodeBody(t, rec, &settings)
	if settings["commissionPct"] != float64(15) {
		t.Fatalf("commissionPct=%v, ждали 15", settings["commissionPct"])
	}

	// Вне 0..90 — 400.
	for _, pct := range []int{-1, 91} {
		rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{
			"commissionPct": pct,
		}, adminCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("commissionPct=%d: status=%d body=%s", pct, rec.Code, rec.Body.String())
		}
	}
	// Без поля — 400.
	rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{}, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустой settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Неизвестное поле — 400 (DisallowUnknownFields: контракт, «золотое правило»).
	rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{
		"commissionPct": 15, "bonus": true,
	}, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("неизвестное поле settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Границы диапазона 0..90 включительно — валидны (только края — 400).
	for _, pct := range []int{0, 90} {
		rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{
			"commissionPct": pct,
		}, adminCookies)
		if rec.Code != http.StatusOK {
			t.Fatalf("commissionPct=%d (граница): status=%d body=%s", pct, rec.Code, rec.Body.String())
		}
		decodeBody(t, rec, &settings)
		if settings["commissionPct"] != float64(pct) {
			t.Fatalf("граница %d: ответ %s", pct, rec.Body.String())
		}
	}

	// Меняем на 25.
	rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{
		"commissionPct": 25,
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &settings)
	if settings["commissionPct"] != float64(25) {
		t.Fatalf("ответ PUT: %s", rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/settings", nil, adminCookies)
	decodeBody(t, rec, &settings)
	if settings["commissionPct"] != float64(25) {
		t.Fatalf("GET после PUT: %s", rec.Body.String())
	}

	// Новая покупка делится по НОВОЙ комиссии: 25% от 10000 = 2500/7500.
	studentCookies, _ := registerUser(t, router, "new-split@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
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
	p, ok := app.Store.GetPaymentByProviderID(enroll["providerPaymentId"].(string))
	if !ok {
		t.Fatal("платёж не найден")
	}
	if p.PlatformFeeCents != 2500 || p.TeacherAmountCents != 7500 {
		t.Fatalf("сплит не учитывает новую комиссию: fee=%d teacher=%d", p.PlatformFeeCents, p.TeacherAmountCents)
	}
}

// Пост-P7: тогл промо-баннера. GET /admin/settings отдаёт promoBarEnabled
// (по умолчанию true), PUT принимает частичное обновление (одно поле вместо
// двух), публичный GET /settings/public отражает состояние без авторизации.
func TestAdminSettingsPromoBar(t *testing.T) {
	app, router := newTestApp()
	// Свой админ (как в seedPublishedCourse): регистрация + роль напрямую в store.
	registerUser(t, router, "promo-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "promo-admin@example.com")
	adminCookies := loginUser(t, router, "promo-admin@example.com", "password123")

	// Публичный эндпоинт доступен без куки и по умолчанию включён.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/settings/public", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("public settings: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var pub map[string]any
	decodeBody(t, rec, &pub)
	if pub["promoBarEnabled"] != true {
		t.Fatalf("promoBarEnabled по умолчанию=%v, ждали true", pub["promoBarEnabled"])
	}
	// Приватные настройки в публичный ответ не попадают.
	if _, ok := pub["commissionPct"]; ok {
		t.Fatalf("публичные настройки не должны отдавать комиссию: %s", rec.Body.String())
	}

	// Админский GET отдаёт оба поля.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/settings", nil, adminCookies)
	var settings map[string]any
	decodeBody(t, rec, &settings)
	if settings["promoBarEnabled"] != true || settings["commissionPct"] != float64(15) {
		t.Fatalf("admin settings: %s", rec.Body.String())
	}

	// Частичный PUT: только промо; комиссия в ответе — текущая (не «нулевая»).
	rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{
		"promoBarEnabled": false,
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("put promo: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &settings)
	if settings["promoBarEnabled"] != false || settings["commissionPct"] != float64(15) {
		t.Fatalf("ответ PUT promo: %s", rec.Body.String())
	}

	// Гость видит выключенный баннер.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/settings/public", nil, nil)
	decodeBody(t, rec, &pub)
	if pub["promoBarEnabled"] != false {
		t.Fatalf("public после выключения: %s", rec.Body.String())
	}

	// Включаем обратно — оба чтения снова true.
	rec = doRequest(t, router, http.MethodPut, "/api/v1/admin/settings", map[string]any{
		"promoBarEnabled": true,
	}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("put promo on: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/settings/public", nil, nil)
	decodeBody(t, rec, &pub)
	if pub["promoBarEnabled"] != true {
		t.Fatalf("public после включения: %s", rec.Body.String())
	}
}
