package contract_test

// Контрактные тесты промокодов (волна P8, T63):
//   - POST/GET/DELETE /teacher/courses/{id}/promo-codes;
//   - POST /courses/{id}/promo/validate;
//   - расширение POST /enrollments (promoCode, amountCents, поля платежа);
//   - идемпотентность usedCount и аддитивные поля журнала админа.
//
// Источник правды — Task/00-api-contract.md, раздел «T63 — промокоды»
// (заморожен). Проверяются дословные тексты ошибок и коды ответов.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// createPromo зовёт POST /teacher/courses/{id}/promo-codes.
func createPromo(t *testing.T, router http.Handler, courseID string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/promo-codes", body, cookies)
}

// seedPromo создаёт промокод через API и возвращает его JSON-представление.
func seedPromo(t *testing.T, router http.Handler, courseID, code string, discountPct, maxUses int, cookies []*http.Cookie) map[string]any {
	t.Helper()
	rec := createPromo(t, router, courseID, map[string]any{
		"code": code, "discountPct": discountPct, "maxUses": maxUses,
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("создание промокода %s: status=%d body=%s", code, rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	promo, _ := out["promoCode"].(map[string]any)
	if promo == nil {
		t.Fatalf("ответ создания %s без promoCode: %s", code, rec.Body.String())
	}
	return promo
}

// promoByCode ищет промокод в сторе по коду (для проверки usedCount/active).
func promoByCode(t *testing.T, app *handler.App, courseID, code string) *repo.PromoCode {
	t.Helper()
	p, ok := app.Store.FindPromoCode(courseID, code)
	if !ok {
		t.Fatalf("промокод %s не найден в сторе", code)
	}
	return p
}

// assertErrText проверяет код и дословный текст ошибки контракта.
func assertErrText(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantText string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status=%d, ждали %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	if got, _ := out["error"].(string); got != wantText {
		t.Fatalf("error=%q, ждали %q; body=%s", got, wantText, rec.Body.String())
	}
}

// TestPromoCodesCreateListRightsAndValidation — создание: нормализация кода,
// значения по умолчанию, права владельца/admin, архив 409, дубликат 400,
// валидация всех полей; список — новые первыми; DELETE идемпотентен.
func TestPromoCodesCreateListRightsAndValidation(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 10000)

	// 401 без cookie.
	rec := createPromo(t, router, courseID, map[string]any{"code": "SALE20", "discountPct": 20}, nil)
	assertErrText(t, rec, http.StatusUnauthorized, "Требуется вход в аккаунт")

	// 403 — чужой курс: второй учитель не владелец.
	otherCookies, _ := registerUser(t, router, "promo-other@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "promo-other@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	rec = createPromo(t, router, courseID, map[string]any{"code": "OTHER20", "discountPct": 20}, otherCookies)
	assertErrText(t, rec, http.StatusForbidden, "Это чужой курс")

	// 404 — курса нет.
	rec = createPromo(t, router, uuid.NewString(), map[string]any{"code": "SALE20", "discountPct": 20}, teacherCookies)
	assertErrText(t, rec, http.StatusNotFound, "Курс не найден")

	// Валидация полей — точные тексты контракта.
	validation := []struct {
		name string
		body map[string]any
		text string
	}{
		{"короткий код", map[string]any{"code": "ab", "discountPct": 20}, "Код: 3–32 символа — латиница, цифры, дефис или подчёркивание"},
		{"пробел в коде", map[string]any{"code": "SALE 20", "discountPct": 20}, "Код: 3–32 символа — латиница, цифры, дефис или подчёркивание"},
		{"кириллица в коде", map[string]any{"code": "САЛЕ20", "discountPct": 20}, "Код: 3–32 символа — латиница, цифры, дефис или подчёркивание"},
		{"скидка 0", map[string]any{"code": "SALE20", "discountPct": 0}, "Скидка должна быть целым числом от 1 до 90"},
		{"скидка 91", map[string]any{"code": "SALE20", "discountPct": 91}, "Скидка должна быть целым числом от 1 до 90"},
		{"отрицательный лимит", map[string]any{"code": "SALE20", "discountPct": 20, "maxUses": -1}, "Лимит использований не может быть отрицательным"},
		{"срок в прошлом", map[string]any{"code": "SALE20", "discountPct": 20, "expiresAt": "2020-01-01T00:00:00Z"}, "Срок действия должен быть в будущем"},
		{"срок не RFC3339", map[string]any{"code": "SALE20", "discountPct": 20, "expiresAt": "31.12.2030"}, "Срок действия должен быть в будущем"},
	}
	for _, tc := range validation {
		t.Run(tc.name, func(t *testing.T) {
			rec := createPromo(t, router, courseID, tc.body, teacherCookies)
			assertErrText(t, rec, http.StatusBadRequest, tc.text)
		})
	}

	// Успешное создание: code trim+upper, expiresAt в будущем.
	rec = createPromo(t, router, courseID, map[string]any{
		"code": " sale20 ", "discountPct": 20, "maxUses": 100,
		"expiresAt": "2030-12-31T23:59:59Z",
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create promo: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		PromoCode map[string]any `json:"promoCode"`
	}
	decodeBody(t, rec, &created)
	promo := created.PromoCode
	if promo == nil {
		t.Fatalf("нет promoCode в ответе: %s", rec.Body.String())
	}
	if promo["code"] != "SALE20" || promo["active"] != true || promo["usedCount"] != float64(0) {
		t.Fatalf("поля созданного кода: %v", promo)
	}
	if promo["courseId"] != courseID || promo["discountPct"] != float64(20) || promo["maxUses"] != float64(100) {
		t.Fatalf("courseId/discountPct/maxUses: %v", promo)
	}
	if promo["expiresAt"] == nil || promo["createdAt"] == nil || promo["id"] == "" {
		t.Fatalf("id/expiresAt/createdAt: %v", promo)
	}

	// Необязательные поля: maxUses=0 (без лимита), expiresAt=null.
	rec = createPromo(t, router, courseID, map[string]any{
		"code": "welcome", "discountPct": 5,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create promo без опций: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Отдельная переменная: json.Unmarshal переиспользует непустую map,
	// и декодирование в created затирало бы id сохранённого выше SALE20.
	var minimal struct {
		PromoCode map[string]any `json:"promoCode"`
	}
	decodeBody(t, rec, &minimal)
	if minimal.PromoCode["code"] != "WELCOME" || minimal.PromoCode["maxUses"] != float64(0) || minimal.PromoCode["expiresAt"] != nil {
		t.Fatalf("необязательные поля: %v", minimal.PromoCode)
	}

	// Дубликат без учёта регистра — 400.
	rec = createPromo(t, router, courseID, map[string]any{"code": "sale20", "discountPct": 10}, teacherCookies)
	assertErrText(t, rec, http.StatusBadRequest, "Такой промокод уже существует на этом курсе")

	// Admin — тоже владелец по правам: создаёт код на чужом курсе.
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	seedPromo(t, router, courseID, "ADMIN10", 10, 0, adminCookies)

	// Список: все коды, новые первыми.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/promo-codes", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("list promo: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &list)
	if len(list.Items) != 3 {
		t.Fatalf("items=%d, ждали 3: %s", len(list.Items), rec.Body.String())
	}
	wantOrder := []string{"ADMIN10", "WELCOME", "SALE20"}
	for i, want := range wantOrder {
		if list.Items[i]["code"] != want {
			t.Fatalf("items[%d].code=%v, ждали %s (новые первыми)", i, list.Items[i]["code"], want)
		}
	}

	// Промокод другого курса через путь этого курса — 404.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Второй курс", "description": "Для проверки прав", "priceCents": 500,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("второй курс: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var second map[string]any
	decodeBody(t, rec, &second)
	secondID, _ := second["id"].(string)
	secondPromo := seedPromo(t, router, secondID, "SECOND", 15, 0, teacherCookies)
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/promo-codes/"+secondPromo["id"].(string), nil, teacherCookies)
	assertErrText(t, rec, http.StatusNotFound, "Промокод не найден")

	// DELETE: деактивация, повтор — 204; запись остаётся в списке.
	saleID, _ := promo["id"].(string)
	for attempt := 1; attempt <= 2; attempt++ {
		rec = doRequest(t, router, http.MethodDelete,
			"/api/v1/teacher/courses/"+courseID+"/promo-codes/"+saleID, nil, teacherCookies)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete promo (попытка %d): status=%d body=%s", attempt, rec.Code, rec.Body.String())
		}
	}
	if got := promoByCode(t, app, courseID, "SALE20"); got.Active {
		t.Fatalf("после DELETE промокод должен быть неактивен: %+v", got)
	}
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/promo-codes/"+uuid.NewString(), nil, teacherCookies)
	assertErrText(t, rec, http.StatusNotFound, "Промокод не найден")

	// Код для проверки деактивации в архиве создаём до архивации: после неё
	// создание запрещено (409).
	adminID, _ := seedPromo(t, router, courseID, "ARCH20", 20, 0, adminCookies)["id"].(string)

	// Архивный курс: создание — 409; чтение и деактивация работают.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/archive", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = createPromo(t, router, courseID, map[string]any{"code": "ARCH10", "discountPct": 10}, teacherCookies)
	assertErrText(t, rec, http.StatusConflict, "Курс в архиве: промокоды недоступны")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/promo-codes", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("list архивного курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/promo-codes/"+adminID, nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete в архиве: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestPromoValidateChecksAndDiscount — проверка кода: скидка вниз до копеек,
// пустой код, отсутствие кода, отключён, истёк, лимит, бесплатный курс,
// неизвестный курс и требование входа.
func TestPromoValidateChecksAndDiscount(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	teacherID := userIDByEmail(t, app.Store, "teacher@example.com")

	seedPromo(t, router, courseID, "SALE20", 20, 0, teacherCookies)
	off := seedPromo(t, router, courseID, "OFF10", 10, 0, teacherCookies)
	rec := doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/promo-codes/"+off["id"].(string), nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("deactivate OFF10: status=%d body=%s", rec.Code, rec.Body.String())
	}
	limit := seedPromo(t, router, courseID, "LIM10", 10, 1, teacherCookies)
	if !app.Store.IncrementPromoCodeUsage(limit["id"].(string)) {
		t.Fatal("не удалось увеличить usedCount LIM10")
	}
	// Истёкший код создаётся напрямую: API не даёт срок в прошлом.
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := app.Store.CreatePromoCode(&repo.PromoCode{
		CourseID: courseID, Code: "OLD10", DiscountPct: 10, ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("создание истёкшего кода: %v", err)
	}

	// Бесплатный курс: создаётся напрямую, без модерации (validate смотрит
	// только существование курса).
	free := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: "promo-free-validate", Title: "Бесплатный",
		Description: "Курс без цены", PriceCents: 0, Currency: "RUB",
	})
	app.Store.SetCourseStatus(free.ID, repo.CoursePublished, "")
	seedPromo(t, router, free.ID, "FREE20", 20, 0, teacherCookies)

	// Курс с ценой не краткой скидке: 9999 − ⌊9999·14/100⌋ = 8600.
	odd := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: "promo-odd-price", Title: "Нечётная цена",
		Description: "Проверка округления", PriceCents: 9999, Currency: "RUB",
	})
	app.Store.SetCourseStatus(odd.ID, repo.CoursePublished, "")
	seedPromo(t, router, odd.ID, "ODD14", 14, 0, teacherCookies)

	studentCookies, _ := registerUser(t, router, "promo-student@example.com", "password123", "")
	validate := func(course string, code any, cookies []*http.Cookie) *httptest.ResponseRecorder {
		return doRequest(t, router, http.MethodPost, "/api/v1/courses/"+course+"/promo/validate",
			map[string]any{"code": code}, cookies)
	}

	// Только вошедший.
	assertErrText(t, validate(courseID, "SALE20", nil), http.StatusUnauthorized, "Требуется вход в аккаунт")

	// Пустой код, неизвестный курс/код — тексты контракта.
	assertErrText(t, validate(courseID, "  ", studentCookies), http.StatusBadRequest, "Укажите промокод")
	assertErrText(t, validate(uuid.NewString(), "SALE20", studentCookies), http.StatusNotFound, "Курс не найден")
	assertErrText(t, validate(courseID, "NOPE", studentCookies), http.StatusBadRequest, "Промокод не найден")
	assertErrText(t, validate(courseID, "OFF10", studentCookies), http.StatusBadRequest, "Промокод отключён")
	assertErrText(t, validate(courseID, "OLD10", studentCookies), http.StatusBadRequest, "Промокод истёк")
	assertErrText(t, validate(courseID, "LIM10", studentCookies), http.StatusBadRequest, "Лимит использований промокода исчерпан")

	// Бесплатный курс: даже чужой/несуществующий код даёт «код не нужен».
	assertErrText(t, validate(free.ID, "SALE20", studentCookies), http.StatusBadRequest, "Курс бесплатный — промокод не нужен")
	assertErrText(t, validate(free.ID, "FREE20", studentCookies), http.StatusBadRequest, "Курс бесплатный — промокод не нужен")

	// Успех: код нормализуется, скидка — в пользу студента.
	var out map[string]any
	rec = validate(courseID, " sale20 ", studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate SALE20: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["code"] != "SALE20" || out["discountPct"] != float64(20) ||
		out["priceCents"] != float64(10000) || out["finalPriceCents"] != float64(8000) {
		t.Fatalf("ответ validate: %v", out)
	}

	// 14% от 9999: 1399.86 → скидка 1399, итог 8600.
	out = nil
	rec = validate(odd.ID, "ODD14", studentCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate ODD14: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &out)
	if out["finalPriceCents"] != float64(8600) {
		t.Fatalf("округление вниз: finalPriceCents=%v, ждали 8600 (%v)", out["finalPriceCents"], out)
	}
}

// TestPromoPurchaseDiscountUsageAndJournal — покупка со скидкой: сумма,
// сплит от фактической суммы, поля платежа; usedCount +1 только при первом
// succeeded; возврат не уменьшает; лимит/ошибки в POST /enrollments; журнал.
func TestPromoPurchaseDiscountUsageAndJournal(t *testing.T) {
	app, router := newTestApp()
	teacherCookies, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	teacherID := userIDByEmail(t, app.Store, "teacher@example.com")
	seedPromo(t, router, courseID, "SALE20", 20, 2, teacherCookies)

	studentCookies, _ := registerUser(t, router, "promo-buyer@example.com", "password123", "")
	enroll := func(cookies []*http.Cookie, code string) *httptest.ResponseRecorder {
		return doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
			"courseId": courseID, "provider": "mock", "promoCode": code,
		}, cookies)
	}

	// Покупка со скидкой: amountCents = finalPriceCents.
	rec := enroll(studentCookies, "sale20")
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll с промокодом: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	decodeBody(t, rec, &created)
	if created["amountCents"] != float64(8000) {
		t.Fatalf("amountCents=%v, ждали 8000 (%s)", created["amountCents"], rec.Body.String())
	}
	providerPaymentID, _ := created["providerPaymentId"].(string)
	p, ok := app.Store.GetPaymentByProviderID(providerPaymentID)
	if !ok {
		t.Fatal("платёж не найден")
	}
	if p.AmountCents != 8000 || p.DiscountCents != 2000 || p.PromoCode != "SALE20" ||
		p.PlatformFeeCents != 1200 || p.TeacherAmountCents != 6800 {
		t.Fatalf("поля платежа со скидкой (сплит от 8000): %+v", p)
	}
	// До успешной оплаты usedCount не растёт.
	if got := promoByCode(t, app, courseID, "SALE20"); got.UsedCount != 0 {
		t.Fatalf("usedCount до оплаты=%d, ждали 0", got.UsedCount)
	}

	webhook := func(pid, status string) *httptest.ResponseRecorder {
		return doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
			"providerPaymentId": pid, "status": status,
		}, nil)
	}
	if rec := webhook(providerPaymentID, "succeeded"); rec.Code != http.StatusOK {
		t.Fatalf("webhook succeeded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := promoByCode(t, app, courseID, "SALE20"); got.UsedCount != 1 {
		t.Fatalf("usedCount после succeeded=%d, ждали 1", got.UsedCount)
	}
	// Повторный вебхук — duplicate и НЕ второй инкремент.
	rec = webhook(providerPaymentID, "succeeded")
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook duplicate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var wh map[string]any
	decodeBody(t, rec, &wh)
	if wh["status"] != "duplicate" {
		t.Fatalf("повторный вебхук: %v", wh)
	}
	if got := promoByCode(t, app, courseID, "SALE20"); got.UsedCount != 1 {
		t.Fatalf("usedCount после duplicate=%d, ждали 1", got.UsedCount)
	}

	// Журнал админа: аддитивные promoCode/discountCents, форма не изменилась.
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/payments?courseId="+courseID, nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("journal: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var journal struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &journal)
	if len(journal.Items) != 1 {
		t.Fatalf("journal items=%d, ждали 1: %s", len(journal.Items), rec.Body.String())
	}
	item := journal.Items[0]
	if item["promoCode"] != "SALE20" || item["discountCents"] != float64(2000) || item["amountCents"] != float64(8000) {
		t.Fatalf("строка журнала: %v", item)
	}

	// Возврат средств: usedCount не уменьшается (контракт T63).
	if rec := webhook(providerPaymentID, "refunded"); rec.Code != http.StatusOK {
		t.Fatalf("webhook refunded: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := promoByCode(t, app, courseID, "SALE20"); got.UsedCount != 1 {
		t.Fatalf("usedCount после возврата=%d, ждали 1 (не уменьшается)", got.UsedCount)
	}

	// Повторная покупка после возврата: новый платёж, usedCount → 2 (лимит).
	rec = enroll(studentCookies, "SALE20")
	if rec.Code != http.StatusCreated {
		t.Fatalf("repurchase: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &created)
	pid2, _ := created["providerPaymentId"].(string)
	if rec := webhook(pid2, "succeeded"); rec.Code != http.StatusOK {
		t.Fatalf("webhook repurchase: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := promoByCode(t, app, courseID, "SALE20"); got.UsedCount != 2 {
		t.Fatalf("usedCount после второй оплаты=%d, ждали 2", got.UsedCount)
	}

	// Лимит исчерпан: следующий ученик получает 400 до создания платежа.
	student2Cookies, student2ID := registerUser(t, router, "promo-buyer2@example.com", "password123", "")
	assertErrText(t, enroll(student2Cookies, "SALE20"), http.StatusBadRequest, "Лимит использований промокода исчерпан")
	if _, pending := app.Store.FindPendingPayment(student2ID, courseID); pending {
		t.Fatal("при ошибке промокода платёж создаваться не должен")
	}

	// Те же тексты ошибок в покупке: неизвестный, отключённый, истёкший код.
	assertErrText(t, enroll(student2Cookies, "NOPE"), http.StatusBadRequest, "Промокод не найден")
	off := seedPromo(t, router, courseID, "OFF10", 10, 0, teacherCookies)
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/teacher/courses/"+courseID+"/promo-codes/"+off["id"].(string), nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("deactivate OFF10: status=%d body=%s", rec.Code, rec.Body.String())
	}
	assertErrText(t, enroll(student2Cookies, "OFF10"), http.StatusBadRequest, "Промокод отключён")
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := app.Store.CreatePromoCode(&repo.PromoCode{
		CourseID: courseID, Code: "OLD10", DiscountPct: 10, ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("создание истёкшего кода: %v", err)
	}
	assertErrText(t, enroll(student2Cookies, "OLD10"), http.StatusBadRequest, "Промокод истёк")

	// Бесплатный курс + промокод — 400, платёж не создаётся.
	free := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: "promo-free-purchase", Title: "Бесплатный",
		Description: "Курс без цены", PriceCents: 0, Currency: "RUB",
	})
	app.Store.SetCourseStatus(free.ID, repo.CoursePublished, "")
	seedPromo(t, router, free.ID, "FREE20", 20, 0, teacherCookies)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": free.ID, "provider": "mock", "promoCode": "FREE20",
	}, student2Cookies)
	assertErrText(t, rec, http.StatusBadRequest, "Курс бесплатный — промокод не нужен")
	if _, pending := app.Store.FindPendingPayment(student2ID, free.ID); pending {
		t.Fatal("бесплатный курс + промокод не должен создавать платёж")
	}

	// Пустая строка promoCode — обычная покупка без скидки (поле необязательно).
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock", "promoCode": "",
	}, student2Cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll без скидки: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &created)
	if created["amountCents"] != float64(10000) {
		t.Fatalf("без промокода amountCents=%v, ждали 10000", created["amountCents"])
	}
}
