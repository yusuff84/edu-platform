package contract_test

// Контрактные тесты T24: раздел «Пользователи» супер-админки —
//   GET    /admin/users?q=&role=&status=&page=&limit=
//   PATCH  /admin/users/{id}/role
//   PATCH  /admin/users/{id}/status
//   DELETE /admin/users/{id}
// плюс эффект блокировки: POST /auth/login → 403 и ранее выданная сессия
// перестаёт пускать (GET /auth/me → 403, requireAuth → 403).
//
// Сценарии 1-8 — по Task/T24-admin-users.md. Сверх списка проверяются:
// порядок «пароль раньше статуса» при входе, каскад записей при удалении
// и гварды ролей на изменяющих маршрутах.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edu-platform/backend/internal/handler"
)

// seedAdminUser регистрирует пользователя и выдаёт ему роль admin напрямую
// в хранилище (как promoteToAdmin): публичная выдача админа — только через
// окружение (seedAdmin в cmd/api).
func seedAdminUser(t *testing.T, app *handler.App, router http.Handler, email string) ([]*http.Cookie, string) {
	t.Helper()
	cookies, id := registerUser(t, router, email, "password123", "")
	promoteToAdmin(t, app, email)
	return cookies, id
}

// getAdminUsersPage вызывает GET /admin/users и разбирает конверт.
// items обязан быть массивом (не null) — проверяется здесь же.
func getAdminUsersPage(t *testing.T, router http.Handler, cookies []*http.Cookie, query string) (map[string]any, []any) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/users"+query, nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/users%s: status=%d body=%s", query, rec.Code, rec.Body.String())
	}
	var page map[string]any
	decodeBody(t, rec, &page)
	items, ok := page["items"].([]any)
	if !ok || items == nil {
		t.Fatalf("items должны быть массивом, не null: %s", rec.Body.String())
	}
	return page, items
}

// adminUserRow ищет строку списка по email.
func adminUserRow(items []any, email string) map[string]any {
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["email"] == email {
			return m
		}
	}
	return nil
}

// assertConflict требует 409 и подстроку в русском тексте ошибки.
func assertConflict(t *testing.T, rec *httptest.ResponseRecorder, wantSub string) {
	t.Helper()
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d, ждали 409: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, wantSub) {
		t.Fatalf("текст ошибки %q не содержит %q", msg, wantSub)
	}
}

// 1. Список: items не null, форма строки, пагинация, поиск по email и имени,
// гварды ролей.
func TestAdminUsersListPaginationAndSearch(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "ul-admin@example.com")

	for i := 0; i < 5; i++ {
		registerUser(t, router, fmt.Sprintf("ul-student%d@example.com", i), "password123", "")
	}
	// displayName — напрямую в хранилище: публичного API правки профиля нет.
	ivanID := userIDByEmail(t, app.Store, "ul-student0@example.com")
	if !app.Store.SetUserDisplayName(ivanID, "Иван Петров") {
		t.Fatal("не удалось задать displayName")
	}

	// Гварды: аноним — 401, студент — 403.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/users", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним GET: status=%d, ждали 401", rec.Code)
	}
	studentCookies, _ := registerUser(t, router, "ul-guard@example.com", "password123", "")
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/users", nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("студент GET: status=%d, ждали 403", rec.Code)
	}

	// Полный список: админ + 5 учеников + guard = 7; значения по умолчанию.
	page, items := getAdminUsersPage(t, router, adminCookies, "")
	if total, _ := page["total"].(float64); total != 7 {
		t.Fatalf("total=%v, ждали 7", page["total"])
	}
	if p, _ := page["page"].(float64); p != 1 {
		t.Fatalf("page=%v, ждали 1", page["page"])
	}
	if l, _ := page["limit"].(float64); l != 20 {
		t.Fatalf("limit=%v, ждали 20", page["limit"])
	}

	// Форма строки — ровно поля контракта.
	row := adminUserRow(items, "ul-student0@example.com")
	if row == nil {
		t.Fatalf("в списке нет ul-student0: %s", items)
	}
	if row["id"] != ivanID {
		t.Fatalf("id=%v, ждали %s", row["id"], ivanID)
	}
	if row["role"] != "student" {
		t.Fatalf("role=%v, ждали student", row["role"])
	}
	if row["status"] != "active" {
		t.Fatalf("status=%v, ждали active", row["status"])
	}
	if row["displayName"] != "Иван Петров" {
		t.Fatalf("displayName=%v", row["displayName"])
	}
	if s, ok := row["createdAt"].(string); !ok || s == "" {
		t.Fatalf("createdAt=%v, ждали RFC3339-строку", row["createdAt"])
	}
	if c, _ := row["coursesCount"].(float64); c != 0 {
		t.Fatalf("coursesCount=%v, ждали 0", row["coursesCount"])
	}
	if e, _ := row["enrollmentsCount"].(float64); e != 0 {
		t.Fatalf("enrollmentsCount=%v, ждали 0", row["enrollmentsCount"])
	}

	// Пагинация: 3+3+1 из 7, страницы не пересекаются.
	seen := map[string]bool{}
	for pageNum, want := range map[int]int{1: 3, 2: 3, 3: 1} {
		p, its := getAdminUsersPage(t, router, adminCookies, fmt.Sprintf("?page=%d&limit=3", pageNum))
		if total, _ := p["total"].(float64); total != 7 {
			t.Fatalf("страница %d: total=%v, ждали 7", pageNum, p["total"])
		}
		if len(its) != want {
			t.Fatalf("страница %d: %d строк, ждали %d", pageNum, len(its), want)
		}
		for _, it := range its {
			id, _ := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("страницы пересекаются по id %s", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("по страницам собралось %d уникальных строк, ждали 7", len(seen))
	}

	// Поиск по email — регистронезависимо.
	_, its := getAdminUsersPage(t, router, adminCookies, "?q=UL-STUDENT3")
	if len(its) != 1 || its[0].(map[string]any)["email"] != "ul-student3@example.com" {
		t.Fatalf("поиск по email: %s", its)
	}
	// Поиск по displayName.
	_, its = getAdminUsersPage(t, router, adminCookies, "?q=петров")
	if len(its) != 1 || its[0].(map[string]any)["id"] != ivanID {
		t.Fatalf("поиск по имени: %s", its)
	}
	// Ничего не найдено: пустой массив (не null), total 0.
	p, its := getAdminUsersPage(t, router, adminCookies, "?q=не-существует")
	if len(its) != 0 {
		t.Fatalf("пустой поиск вернул строки: %s", its)
	}
	if total, _ := p["total"].(float64); total != 0 {
		t.Fatalf("пустой поиск: total=%v", p["total"])
	}
}

// 2. Фильтры по роли и статусу; мусорные значения — 400.
func TestAdminUsersRoleAndStatusFilters(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "flt-admin@example.com")
	registerUser(t, router, "flt-teacher@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "flt-teacher@example.com"), "teacher") {
		t.Fatal("не удалось выдать роль teacher")
	}
	_, studentID := registerUser(t, router, "flt-student@example.com", "password123", "")

	// role — точное совпадение.
	for role, wantEmail := range map[string]string{
		"admin":   "flt-admin@example.com",
		"teacher": "flt-teacher@example.com",
		"student": "flt-student@example.com",
	} {
		_, items := getAdminUsersPage(t, router, adminCookies, "?role="+role)
		if len(items) != 1 || items[0].(map[string]any)["email"] != wantEmail {
			t.Fatalf("role=%s: %s", role, items)
		}
	}

	// До блокировки все трое активны.
	_, items := getAdminUsersPage(t, router, adminCookies, "?status=active")
	if len(items) != 3 {
		t.Fatalf("status=active до блокировки: %d строк, ждали 3", len(items))
	}

	// Блокируем ученика через API.
	rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/status",
		map[string]any{"status": "blocked"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("блокировка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	user, _ := out["user"].(map[string]any)
	if user == nil || user["status"] != "blocked" {
		t.Fatalf("ответ блокировки без user.status=blocked: %s", rec.Body.String())
	}

	// status=blocked — только заблокированный; active — двое.
	_, items = getAdminUsersPage(t, router, adminCookies, "?status=blocked")
	if len(items) != 1 || items[0].(map[string]any)["id"] != studentID {
		t.Fatalf("status=blocked: %s", items)
	}
	_, items = getAdminUsersPage(t, router, adminCookies, "?status=active")
	if len(items) != 2 {
		t.Fatalf("status=active после блокировки: %d строк, ждали 2", len(items))
	}
	// Комбинация фильтров.
	_, items = getAdminUsersPage(t, router, adminCookies, "?role=student&status=blocked")
	if len(items) != 1 || items[0].(map[string]any)["id"] != studentID {
		t.Fatalf("role=student&status=blocked: %s", items)
	}

	// Мусорные значения — 400.
	for _, qs := range []string{"?role=manager", "?status=banned", "?page=abc", "?limit=abc"} {
		rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/users"+qs, nil, adminCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d, ждали 400: %s", qs, rec.Code, rec.Body.String())
		}
	}
}

// 3. Смена роли student → teacher; идемпотентность, 400/404, гвард роли.
func TestAdminUserRoleChangePersists(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "role-admin@example.com")
	_, studentID := registerUser(t, router, "role-up@example.com", "password123", "")
	path := "/api/v1/admin/users/" + studentID + "/role"

	// student → teacher: 200 и форма {user:{...}}.
	rec := doRequest(t, router, http.MethodPatch, path, map[string]any{"role": "teacher"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("смена роли: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	decodeBody(t, rec, &out)
	user, _ := out["user"].(map[string]any)
	if user == nil || user["role"] != "teacher" || user["id"] != studentID {
		t.Fatalf("ответ смены роли: %s", rec.Body.String())
	}

	// Роль реально изменилась — видно в списке.
	_, items := getAdminUsersPage(t, router, adminCookies, "?q=role-up")
	if len(items) != 1 || items[0].(map[string]any)["role"] != "teacher" {
		t.Fatalf("роль в списке не изменилась: %s", items)
	}

	// Идемпотентность: та же роль — снова 200.
	rec = doRequest(t, router, http.MethodPatch, path, map[string]any{"role": "teacher"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("повторная роль: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 400: недопустимая роль, пустое тело, лишнее поле (DisallowUnknownFields).
	for _, body := range []map[string]any{
		{"role": "manager"}, {}, {"role": "teacher", "status": "blocked"},
	} {
		rec = doRequest(t, router, http.MethodPatch, path, body, adminCookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("тело %v: status=%d, ждали 400", body, rec.Code)
		}
	}

	// 404: пользователя нет.
	rec = doRequest(t, router, http.MethodPatch,
		"/api/v1/admin/users/00000000-0000-0000-0000-000000000000/role",
		map[string]any{"role": "teacher"}, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("несуществующий пользователь: status=%d, ждали 404", rec.Code)
	}

	// Не-админ — 403 ещё до обработчика (requireRole).
	studentCookies := loginUser(t, router, "role-up@example.com", "password123")
	rec = doRequest(t, router, http.MethodPatch, path, map[string]any{"role": "student"}, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("учитель на смену роли: status=%d, ждали 403", rec.Code)
	}
}

// 4. Защита от запирания себя: своя роль, своя блокировка, своё удаление —
// 409. Проверяется при ДВУХ админах: срабатывает именно защита «себя»,
// а не последнего администратора.
func TestAdminUsersSelfProtection(t *testing.T) {
	app, router := newTestApp()
	adminCookies, adminID := seedAdminUser(t, app, router, "self-a@example.com")
	_, otherID := seedAdminUser(t, app, router, "self-b@example.com")

	rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+adminID+"/role",
		map[string]any{"role": "student"}, adminCookies)
	assertConflict(t, rec, "Нельзя менять собственную роль")

	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+adminID+"/status",
		map[string]any{"status": "blocked"}, adminCookies)
	assertConflict(t, rec, "Нельзя заблокировать собственную учётную запись")

	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+adminID, nil, adminCookies)
	assertConflict(t, rec, "Нельзя удалить собственную учётную запись")

	// Админ не пострадал и продолжает работать.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, adminCookies); rec.Code != http.StatusOK {
		t.Fatalf("me после отказов: status=%d", rec.Code)
	}

	// Чужого админа при двух админах разжаловать можно.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+otherID+"/role",
		map[string]any{"role": "student"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("разжалование второго админа: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Теперь adminID — последний администратор: своё действие отклоняет
	// уже защита последнего (сообщение — про последнего администратора).
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+adminID+"/role",
		map[string]any{"role": "teacher"}, adminCookies)
	assertConflict(t, rec, "Нельзя снять роль администратора с последнего администратора")
}

// 5. Последний администратор: разжаловать/заблокировать/удалить → 409,
// состояние не меняется.
func TestAdminUsersLastAdminProtection(t *testing.T) {
	app, router := newTestApp()
	adminCookies, adminID := seedAdminUser(t, app, router, "last-admin@example.com")

	// В системе ровно один администратор — он сам.
	rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+adminID+"/role",
		map[string]any{"role": "student"}, adminCookies)
	assertConflict(t, rec, "Нельзя снять роль администратора с последнего администратора")

	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+adminID+"/status",
		map[string]any{"status": "blocked"}, adminCookies)
	assertConflict(t, rec, "Нельзя заблокировать последнего администратора")

	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+adminID, nil, adminCookies)
	assertConflict(t, rec, "Нельзя удалить последнего администратора")

	// Админ по-прежнему администратор, активен и продолжает работать.
	_, items := getAdminUsersPage(t, router, adminCookies, "?q=last-admin")
	if len(items) != 1 {
		t.Fatalf("админ пропал из списка: %s", items)
	}
	row := items[0].(map[string]any)
	if row["role"] != "admin" || row["status"] != "active" {
		t.Fatalf("состояние админа изменилось: %s", row)
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, adminCookies); rec.Code != http.StatusOK {
		t.Fatalf("me последнего админа: status=%d", rec.Code)
	}
}

// 6. Блокировка ученика: вход → 403, ранее выданная сессия → 403;
// разблокировка возвращает доступ, сессия не удалялась.
func TestAdminUsersBlockedUserLosesAccess(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "blk-admin@example.com")
	studentCookies, studentID := registerUser(t, router, "blk-student@example.com", "password123", "")

	// До блокировки сессия и вход работают.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, studentCookies); rec.Code != http.StatusOK {
		t.Fatalf("me до блокировки: status=%d", rec.Code)
	}

	// Блокируем ученика.
	rec := doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/status",
		map[string]any{"status": "blocked"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("блокировка: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Вход с верным паролем — 403 с русским текстом.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "blk-student@example.com", "password": "password123",
	}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("вход заблокированного: status=%d, ждали 403: %s", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if msg, _ := errBody["error"].(string); !strings.Contains(msg, "заблокирована") {
		t.Fatalf("текст 403: %q", errBody["error"])
	}

	// Неверный пароль у заблокированного — всё ещё 401: пароль проверяется
	// раньше статуса, 403 не раскрывает существование чужого аккаунта.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "blk-student@example.com", "password": "wrong-password",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("неверный пароль: status=%d, ждали 401", rec.Code)
	}

	// Ранее выданная сессия перестаёт пускать.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("me заблокированного: status=%d, ждали 403", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("me/courses заблокированного: status=%d, ждали 403", rec.Code)
	}

	// Повторная блокировка идемпотентна.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/status",
		map[string]any{"status": "blocked"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("повторная блокировка: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Разблокировка: вход и старая сессия снова работают (сессии не удалялись).
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/status",
		map[string]any{"status": "active"}, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("разблокировка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	loginUser(t, router, "blk-student@example.com", "password123")
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, studentCookies); rec.Code != http.StatusOK {
		t.Fatalf("старая сессия после разблокировки: status=%d", rec.Code)
	}

	// Валидация тела: мусорный статус — 400.
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/admin/users/"+studentID+"/status",
		map[string]any{"status": "banned"}, adminCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("мусорный статус: status=%d, ждали 400", rec.Code)
	}
	// 404: нет пользователя.
	rec = doRequest(t, router, http.MethodPatch,
		"/api/v1/admin/users/00000000-0000-0000-0000-000000000000/status",
		map[string]any{"status": "blocked"}, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("нет пользователя: status=%d, ждали 404", rec.Code)
	}
}

// 7. Удаление свободного пользователя: 204, пропадает из списка, сессия
// мертва; повторное удаление — 404.
func TestAdminUsersDeleteUnencumberedUser(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "del-admin@example.com")
	studentCookies, studentID := registerUser(t, router, "trash@example.com", "password123", "")

	// Гварды: аноним — 401, студент — 403.
	if rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+studentID, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним DELETE: status=%d, ждали 401", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+studentID, nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("студент DELETE: status=%d, ждали 403", rec.Code)
	}

	// Удаление: 204 без тела.
	rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+studentID, nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 не должен содержать тело: %s", rec.Body.String())
	}

	// Пропал из списка.
	_, items := getAdminUsersPage(t, router, adminCookies, "?q=trash")
	if len(items) != 0 {
		t.Fatalf("удалённый остался в списке: %s", items)
	}

	// Сессия удалённого мертва — 401.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, studentCookies); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me удалённого: status=%d, ждали 401", rec.Code)
	}

	// Повторное удаление и несуществующий UUID — 404.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+studentID, nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("повторное удаление: status=%d, ждали 404", rec.Code)
	}
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/admin/users/00000000-0000-0000-0000-000000000000", nil, adminCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("несуществующий UUID: status=%d, ждали 404", rec.Code)
	}
	_ = app
}

// 8. Удаление с зависимостями: курс преподавателя → 409; платёж → 409;
// записи на курс не мешают удалению (чистятся каскадом).
func TestAdminUsersDeleteRestrictions(t *testing.T) {
	app, router := newTestApp()
	_, courseID, _ := seedPublishedCourse(t, app, router, 10000)
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	// Преподаватель с курсом — 409.
	teacherID := userIDByEmail(t, app.Store, "teacher@example.com")
	rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+teacherID, nil, adminCookies)
	assertConflict(t, rec, "Нельзя удалить пользователя с курсами")

	// Ученик с платежом — 409 (история оплат сохраняется).
	studentCookies, studentID := registerUser(t, router, "paid@example.com", "password123", "")
	rec = doRequest(t, router, http.MethodPost, "/api/v1/enrollments", map[string]any{
		"courseId": courseID, "provider": "mock",
	}, studentCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("покупка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var enroll map[string]any
	decodeBody(t, rec, &enroll)
	rec = doRequest(t, router, http.MethodPost, "/api/v1/payments/webhook/mock", map[string]any{
		"providerPaymentId": enroll["providerPaymentId"], "status": "succeeded",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+studentID, nil, adminCookies)
	assertConflict(t, rec, "история оплат должна сохраниться")

	// Ученик без платежа, но с записью на курс: записи не мешают удалению —
	// каскад чистит их (запись создаётся напрямую в хранилище).
	_, cloudID := registerUser(t, router, "cloud@example.com", "password123", "")
	app.Store.EnsureEnrollment(cloudID, courseID)
	if deps := app.Store.UserDependencies(cloudID); deps.Payments != 0 || deps.Enrollments != 1 {
		t.Fatalf("подготовка: deps=%+v", deps)
	}
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+cloudID, nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("удаление пользователя с записью: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := app.Store.GetEnrollment(cloudID, courseID); ok {
		t.Fatal("запись на курс не удалена каскадом")
	}

	// Провалившиеся удаления ничего не испортили.
	if _, ok := app.Store.GetUserByID(teacherID); !ok {
		t.Fatal("преподаватель удалён несмотря на 409")
	}
	if _, ok := app.Store.GetUserByID(studentID); !ok {
		t.Fatal("ученик с платежом удалён несмотря на 409")
	}
}
