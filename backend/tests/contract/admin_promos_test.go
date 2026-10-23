package contract_test

// Контрактные тесты T89 — глобальный журнал промокодов супер-админа:
//   GET  /api/v1/admin/promo-codes  — фильтры courseId/q/active, пагинация,
//                                     total, форма AdminPromo (поля PromoCode +
//                                     courseTitle/courseSlug);
//   PATCH /api/v1/admin/promo-codes/{id} — частичное обновление каждого поля,
//                                     очистка expiresAt, 400/404/409/401/403.
//
// Источник правды — Task/00-api-contract.md, раздел «T89 — глобальное
// управление промокодами» (заморожен). Коды проверяются на in-memory Store;
// SQL-ветка — в tests/postgres/admin_promos_test.go.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// adminPromoListBody — ответ GET /admin/promo-codes в форме контракта T89.
type adminPromoListBody struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
}

// seedAdminPromo создаёт промокод напрямую в Store: так тест управляет
// active/expiresAt/usedCount, не обходя проверки teacher-API.
func seedAdminPromo(t *testing.T, app *handler.App, courseID, code string, discountPct, maxUses int, active bool, expiresAt *time.Time) *repo.PromoCode {
	t.Helper()
	p, err := app.Store.CreatePromoCode(&repo.PromoCode{
		CourseID: courseID, Code: code, DiscountPct: discountPct,
		MaxUses: maxUses, ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("создание промокода %s: %v", code, err)
	}
	if !active {
		if err := app.Store.DeactivatePromoCode(p.ID); err != nil {
			t.Fatalf("деактивация промокода %s: %v", code, err)
		}
	}
	return p
}

// getAdminPromoList зовёт журнал и декодирует контрактный конверт.
func getAdminPromoList(t *testing.T, router http.Handler, query string, cookies []*http.Cookie) adminPromoListBody {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/promo-codes"+query, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET promo-codes%s: status=%d body=%s", query, rec.Code, rec.Body.String())
	}
	var out adminPromoListBody
	decodeBody(t, rec, &out)
	return out
}

// patchAdminPromo зовёт PATCH и возвращает recorder: тесты проверяют и успех,
// и коды ошибок с текстами.
func patchAdminPromo(t *testing.T, router http.Handler, promoID string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, router, http.MethodPatch, "/api/v1/admin/promo-codes/"+promoID, body, cookies)
}

// TestAdminPromoCodesListFilters — журнал: форма строки по контракту, фильтр
// по курсу, поиск q по коду и названию курса без учёта регистра, active,
// пагинация/total, пустой items — [], права и 400 на неверные query.
func TestAdminPromoCodesListFilters(t *testing.T) {
	app, router := newTestApp()
	_, course1, slug1 := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	teacherID := userIDByEmail(t, app.Store, "teacher@example.com")

	// Второй курс — вручную: нужен только для фильтра по курсу и поиска.
	course2 := app.Store.CreateCourse(&repo.Course{
		TeacherID: teacherID, Slug: "t89-second-course", Title: "Python с нуля",
		Description: "Второй курс T89", PriceCents: 5000, Currency: "RUB",
	})
	if course2 == nil || course2.ID == "" {
		t.Fatal("не удалось создать второй курс")
	}

	expires := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Microsecond)
	p1 := seedAdminPromo(t, app, course1, "SALE20", 20, 100, true, nil)
	// Небольшая пауза не нужна: in-memory Store выдаёт монотонный createdAt.
	p2 := seedAdminPromo(t, app, course1, "OLD10", 10, 0, false, &expires)
	p3 := seedAdminPromo(t, app, course2.ID, "SUMMER50", 50, 200, true, nil)

	// Без фильтров: все коды, новые первыми, total до пагинации.
	list := getAdminPromoList(t, router, "", adminCookies)
	if list.Total != 3 || len(list.Items) != 3 || list.Page != 1 || list.Limit != 20 {
		t.Fatalf("журнал без фильтров: %+v", list)
	}
	wantOrder := []string{p3.Code, p2.Code, p1.Code}
	for i, want := range wantOrder {
		if list.Items[i]["code"] != want {
			t.Fatalf("items[%d].code=%v, ждали %s (новые первыми)", i, list.Items[i]["code"], want)
		}
	}

	// Форма элемента: все поля контракта + данные курса.
	item := list.Items[2] // SALE20 (первый созданный)
	for _, key := range []string{
		"id", "courseId", "courseTitle", "courseSlug", "code", "discountPct",
		"maxUses", "usedCount", "expiresAt", "active", "createdAt",
	} {
		if _, ok := item[key]; !ok {
			t.Fatalf("в строке журнала нет поля %q: %v", key, item)
		}
	}
	if item["id"] != p1.ID || item["courseId"] != course1 || item["code"] != "SALE20" ||
		item["discountPct"] != float64(20) || item["maxUses"] != float64(100) ||
		item["usedCount"] != float64(0) || item["expiresAt"] != nil || item["active"] != true {
		t.Fatalf("поля строки журнала: %v", item)
	}
	if item["courseTitle"] != "Go для новичков" || item["courseSlug"] != slug1 {
		t.Fatalf("данные курса в строке: title=%v slug=%v, ждали %q/%q",
			item["courseTitle"], item["courseSlug"], "Go для новичков", slug1)
	}
	if item["createdAt"] == nil {
		t.Fatalf("createdAt не заполнен: %v", item)
	}
	// Срок у OLD10 отдаётся как RFC3339 (не строка-объект), active=false.
	if list.Items[1]["expiresAt"] == nil || list.Items[1]["active"] != false {
		t.Fatalf("строка OLD10: %v", list.Items[1])
	}

	// Фильтр по курсу.
	list = getAdminPromoList(t, router, "?courseId="+course1, adminCookies)
	if list.Total != 2 || len(list.Items) != 2 ||
		list.Items[0]["code"] != "OLD10" || list.Items[1]["code"] != "SALE20" {
		t.Fatalf("courseId=%s: %+v", course1, list)
	}

	// q по коду без учёта регистра.
	list = getAdminPromoList(t, router, "?q=sale", adminCookies)
	if list.Total != 1 || list.Items[0]["code"] != "SALE20" {
		t.Fatalf("q=sale: %+v", list)
	}
	// q по названию курса без учёта регистра: оба кода первого курса.
	list = getAdminPromoList(t, router, "?q=%D0%BD%D0%BE%D0%B2%D0%B8%D1%87%D0%BA%D0%BE%D0%B2", adminCookies) // «новичков»
	if list.Total != 2 || len(list.Items) != 2 {
		t.Fatalf("q по названию курса: %+v", list)
	}
	list = getAdminPromoList(t, router, "?q=python", adminCookies)
	if list.Total != 1 || list.Items[0]["code"] != "SUMMER50" {
		t.Fatalf("q=python: %+v", list)
	}
	// Ничего не найдено: items — пустой массив, не null.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/promo-codes?q=nothing-matches", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("пустой q: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var empty map[string]any
	decodeBody(t, rec, &empty)
	if items, ok := empty["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("пустой результат должен быть [], got %s", rec.Body.String())
	}

	// active=true|false.
	list = getAdminPromoList(t, router, "?active=true", adminCookies)
	if list.Total != 2 {
		t.Fatalf("active=true: total=%d", list.Total)
	}
	list = getAdminPromoList(t, router, "?active=false", adminCookies)
	if list.Total != 1 || list.Items[0]["code"] != "OLD10" {
		t.Fatalf("active=false: %+v", list)
	}
	// Фильтры комбинируются через AND.
	list = getAdminPromoList(t, router, "?courseId="+course1+"&active=true", adminCookies)
	if list.Total != 1 || list.Items[0]["code"] != "SALE20" {
		t.Fatalf("courseId+active: %+v", list)
	}

	// Пагинация: total не зависит от страницы, limit обрезается до 100.
	list = getAdminPromoList(t, router, "?page=1&limit=1", adminCookies)
	if list.Total != 3 || len(list.Items) != 1 || list.Page != 1 || list.Limit != 1 ||
		list.Items[0]["code"] != "SUMMER50" {
		t.Fatalf("page=1&limit=1: %+v", list)
	}
	list = getAdminPromoList(t, router, "?page=2&limit=1", adminCookies)
	if list.Total != 3 || len(list.Items) != 1 || list.Items[0]["code"] != "OLD10" {
		t.Fatalf("page=2&limit=1: %+v", list)
	}
	list = getAdminPromoList(t, router, "?page=99&limit=1", adminCookies)
	if list.Total != 3 || len(list.Items) != 0 {
		t.Fatalf("page=99: %+v", list)
	}
	list = getAdminPromoList(t, router, "?limit=500", adminCookies)
	if list.Limit != 100 {
		t.Fatalf("limit=500 должен обрезаться до 100, got %d", list.Limit)
	}
	list = getAdminPromoList(t, router, "?page=0", adminCookies)
	if list.Page != 1 {
		t.Fatalf("page=0 должен стать 1, got %d", list.Page)
	}

	// Неверные query — 400.
	for _, tc := range []struct {
		name  string
		query string
		text  string
	}{
		{"active не bool", "?active=yes", "Параметр active должен быть true или false"},
		{"courseId не UUID", "?courseId=not-a-uuid", "Параметр courseId должен быть UUID"},
		{"page не число", "?page=abc", "Параметры page и limit должны быть целыми числами"},
		{"limit не число", "?limit=abc", "Параметры page и limit должны быть целыми числами"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/promo-codes"+tc.query, nil, adminCookies)
			assertErrText(t, rec, http.StatusBadRequest, tc.text)
		})
	}

	// 401 без cookie, 403 — студент (не admin), даже с валидной сессией.
	assertErrText(t,
		doRequest(t, router, http.MethodGet, "/api/v1/admin/promo-codes", nil, nil),
		http.StatusUnauthorized, "Требуется вход в аккаунт")
	studentCookies, _ := registerUser(t, router, "promo-list-student@example.com", "password123", "")
	assertErrText(t,
		doRequest(t, router, http.MethodGet, "/api/v1/admin/promo-codes", nil, studentCookies),
		http.StatusForbidden, "Недостаточно прав для этого действия")
}

// TestAdminPromoCodesUpdatePartialAndValidation — PATCH: частичное обновление
// каждого поля, очистка срока, включение/отключение, использованный лимит,
// 400 (пустое тело/{} / неизвестное поле/значения), 404, 409 archived, 401/403.
func TestAdminPromoCodesUpdatePartialAndValidation(t *testing.T) {
	app, router := newTestApp()
	_, courseID, slug := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")
	promo := seedAdminPromo(t, app, courseID, "PATCH20", 20, 100, true, nil)

	// 401/403.
	assertErrText(t, patchAdminPromo(t, router, promo.ID, map[string]any{"active": false}, nil),
		http.StatusUnauthorized, "Требуется вход в аккаунт")
	studentCookies, _ := registerUser(t, router, "promo-patch-student@example.com", "password123", "")
	assertErrText(t, patchAdminPromo(t, router, promo.ID, map[string]any{"active": false}, studentCookies),
		http.StatusForbidden, "Недостаточно прав для этого действия")

	// Пустое тело и {} — 400; неизвестное поле — 400 (DisallowUnknownFields).
	rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/promo-codes/"+promo.ID, nil, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустое тело: status=%d body=%s", rec.Code, rec.Body.String())
	}
	assertErrText(t, patchAdminPromo(t, router, promo.ID, map[string]any{}, adminCookies),
		http.StatusBadRequest, "Укажите хотя бы одно поле для обновления")
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"unknown": 1}, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("неизвестное поле: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Неверные значения — 400 и состояние не меняется.
	validation := []struct {
		name string
		body map[string]any
		text string
	}{
		{"скидка 0", map[string]any{"discountPct": 0}, "Скидка должна быть целым числом от 1 до 90"},
		{"скидка 91", map[string]any{"discountPct": 91}, "Скидка должна быть целым числом от 1 до 90"},
		{"лимит отрицательный", map[string]any{"maxUses": -1}, "Лимит использований не может быть отрицательным"},
		{"срок в прошлом", map[string]any{"expiresAt": "2020-01-01T00:00:00Z"}, "Срок действия должен быть в будущем"},
		{"срок не RFC3339", map[string]any{"expiresAt": "31.12.2030"}, "Срок действия должен быть в будущем"},
		{"срок не строка", map[string]any{"expiresAt": 12345}, "Срок действия должен быть в будущем"},
	}
	for _, tc := range validation {
		t.Run(tc.name, func(t *testing.T) {
			assertErrText(t, patchAdminPromo(t, router, promo.ID, tc.body, adminCookies),
				http.StatusBadRequest, tc.text)
		})
	}
	if got := promoByCode(t, app, courseID, "PATCH20"); got.DiscountPct != 20 || got.MaxUses != 100 {
		t.Fatalf("после ошибок валидации код изменился: %+v", got)
	}

	// Частичные обновления каждого поля: остальные значения сохраняются.
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"discountPct": 25}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH discountPct: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updated struct {
		PromoCode map[string]any `json:"promoCode"`
	}
	decodeBody(t, rec, &updated)
	if updated.PromoCode["discountPct"] != float64(25) ||
		updated.PromoCode["maxUses"] != float64(100) ||
		updated.PromoCode["active"] != true ||
		updated.PromoCode["courseSlug"] != slug ||
		updated.PromoCode["courseTitle"] != "Go для новичков" {
		t.Fatalf("ответ PATCH discountPct: %v", updated.PromoCode)
	}

	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"maxUses": 200}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH maxUses: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &updated)
	if updated.PromoCode["maxUses"] != float64(200) || updated.PromoCode["discountPct"] != float64(25) {
		t.Fatalf("ответ PATCH maxUses: %v", updated.PromoCode)
	}

	// Срок: установка, затем очистка явным null.
	future := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"expiresAt": future.Format(time.RFC3339)}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH expiresAt: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &updated)
	if updated.PromoCode["expiresAt"] == nil {
		t.Fatalf("expiresAt не установлен: %v", updated.PromoCode)
	}
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"expiresAt": nil}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH expiresAt=null: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &updated)
	if updated.PromoCode["expiresAt"] != nil {
		t.Fatalf("expiresAt не очищен: %v", updated.PromoCode)
	}

	// Отключение не удаляет код: он остаётся в журнале с active=false;
	// повторное включение возвращает active=true.
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"active": false}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH active=false: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &updated)
	if updated.PromoCode["active"] != false {
		t.Fatalf("active=false не применился: %v", updated.PromoCode)
	}
	list := getAdminPromoList(t, router, "?courseId="+courseID+"&active=false", adminCookies)
	if list.Total != 1 || list.Items[0]["id"] != promo.ID {
		t.Fatalf("отключённый код пропал из журнала: %+v", list)
	}
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"active": true}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH active=true: status=%d body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &updated)
	if updated.PromoCode["active"] != true {
		t.Fatalf("active=true не применился: %v", updated.PromoCode)
	}

	// Неизвестный id (и мусорный) — 404.
	assertErrText(t, patchAdminPromo(t, router, uuid.NewString(), map[string]any{"active": false}, adminCookies),
		http.StatusNotFound, "Промокод не найден")
	assertErrText(t, patchAdminPromo(t, router, "not-a-uuid", map[string]any{"active": false}, adminCookies),
		http.StatusNotFound, "Промокод не найден")

	// Использованный лимит: usedCount=3; maxUses=3 разрешён, maxUses=2 — 400.
	for i := 0; i < 3; i++ {
		if !app.Store.IncrementPromoCodeUsage(promo.ID) {
			t.Fatal("IncrementPromoCodeUsage вернул false")
		}
	}
	rec = patchAdminPromo(t, router, promo.ID, map[string]any{"maxUses": 3}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("maxUses=usedCount: status=%d body=%s", rec.Code, rec.Body.String())
	}
	assertErrText(t, patchAdminPromo(t, router, promo.ID, map[string]any{"maxUses": 2}, adminCookies),
		http.StatusBadRequest, "Лимит использований не может быть меньше использованного количества")
	if got := promoByCode(t, app, courseID, "PATCH20"); got.MaxUses != 3 || got.UsedCount != 3 {
		t.Fatalf("после лимита: %+v", got)
	}

	// Архивный курс: 409 и при валидном теле, и при пустом (guard до decode).
	if !app.Store.SetCourseStatus(courseID, repo.CourseArchived, "") {
		t.Fatal("не удалось перевести курс в архив")
	}
	assertErrText(t, patchAdminPromo(t, router, promo.ID, map[string]any{"discountPct": 10}, adminCookies),
		http.StatusConflict, "Курс в архиве: промокоды недоступны")
	assertErrText(t, patchAdminPromo(t, router, promo.ID, map[string]any{}, adminCookies),
		http.StatusConflict, "Курс в архиве: промокоды недоступны")
	if got := promoByCode(t, app, courseID, "PATCH20"); got.DiscountPct != 25 {
		t.Fatalf("архивный PATCH изменил код: %+v", got)
	}

	// Архивный код читать можно: журнал не скрывает архивные курсы.
	list = getAdminPromoList(t, router, "?courseId="+courseID, adminCookies)
	if list.Total != 1 || list.Items[0]["id"] != promo.ID {
		t.Fatalf("журнал архивного курса: %+v", list)
	}
}
