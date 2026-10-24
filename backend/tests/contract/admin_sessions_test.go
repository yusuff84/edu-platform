package contract_test

// Контрактные тесты T93: сессии пользователя и принудительный выход —
//   GET    /api/v1/admin/users/{id}/sessions
//   DELETE /api/v1/admin/users/{id}/sessions
//
// Проверяются: вход создаёт сессию, админ видит её (только не истёкшие,
// новые первыми, items всегда массив); после DELETE старые cookie не проходят
// requireAuth; чужие сессии не затрагиваются; 401/403/404; удаление
// собственных сессий администратором.

import (
	"net/http"
	"testing"
	"time"
)

// t93SessionItem — элемент контрактного ответа T93.
type t93SessionItem struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// t93SessionsPage — конверт {items:[...]}.
type t93SessionsPage struct {
	Items []t93SessionItem `json:"items"`
}

// t93GetSessions вызывает GET /admin/users/{id}/sessions и проверяет, что
// ответ — 200 и items не null. Возвращает разобранный конверт и «сырые»
// строки для проверки точного набора полей.
func t93GetSessions(t *testing.T, router http.Handler, cookies []*http.Cookie, userID string) (t93SessionsPage, []map[string]any) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/admin/users/"+userID+"/sessions", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET сессий %s: status=%d body=%s", userID, rec.Code, rec.Body.String())
	}
	var page t93SessionsPage
	decodeBody(t, rec, &page)
	if page.Items == nil {
		t.Fatalf("items должны быть массивом, не null: %s", rec.Body.String())
	}
	var raw struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &raw)
	return page, raw.Items
}

// t93AssertSessionShape проверяет поля одного элемента: id непустой,
// expiresAt в будущем и позже createdAt, набор ключей — ровно контрактный
// (без id пользователя и без токенов/подписей).
func t93AssertSessionShape(t *testing.T, item t93SessionItem, raw map[string]any, expiredID string) {
	t.Helper()
	if item.ID == "" {
		t.Fatal("id сессии пуст")
	}
	if item.ID == expiredID {
		t.Fatalf("истёкшая сессия %s попала в ответ", expiredID)
	}
	if !item.ExpiresAt.After(item.CreatedAt) {
		t.Fatalf("expiresAt %v не позже createdAt %v", item.ExpiresAt, item.CreatedAt)
	}
	if !item.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("в ответе истёкшая сессия: expiresAt=%v", item.ExpiresAt)
	}
	want := map[string]bool{"id": true, "createdAt": true, "expiresAt": true}
	for key := range raw {
		if !want[key] {
			t.Fatalf("лишнее поле сессии в JSON: %q (%v)", key, raw)
		}
	}
	for key := range want {
		if _, ok := raw[key]; !ok {
			t.Fatalf("в элементе сессии нет поля %q: %v", key, raw)
		}
	}
}

// TestAdminUserSessionsListAndRevoke — основной сценарий: активные сессии
// видны админу, истёкшая не возвращается, после DELETE обе cookie мёртвы,
// сессии других пользователей и админа живы, повторный вызов — 204.
func TestAdminUserSessionsListAndRevoke(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "sess-admin@example.com")
	targetCookies, targetID := registerUser(t, router, "sess-target@example.com", "password123", "")
	// Пауза больше тика системных часов Windows (~15 мс): createdAt сессий
	// должны различаться, иначе проверить порядок «новые первыми» нельзя.
	time.Sleep(20 * time.Millisecond)
	targetCookies2 := loginUser(t, router, "sess-target@example.com", "password123")
	otherCookies, otherID := registerUser(t, router, "sess-other@example.com", "password123", "")

	// Истёкшая сессия — напрямую в store (короткий TTL): контракт требует
	// не показывать её, хотя физически запись ещё существует.
	expired := app.Store.CreateSession(targetID, -time.Minute)
	if expired.ID == "" {
		t.Fatal("CreateSession с отрицательным TTL не вернул id")
	}

	page, raw := t93GetSessions(t, router, adminCookies, targetID)
	if len(page.Items) != 2 {
		t.Fatalf("активных сессий %d, ждали 2 (регистрация + вход): %+v", len(page.Items), page.Items)
	}
	if len(raw) != len(page.Items) {
		t.Fatalf("сырых элементов %d, разобранных %d", len(raw), len(page.Items))
	}
	for i, item := range page.Items {
		t93AssertSessionShape(t, item, raw[i], expired.ID)
		// Новые первыми: время создания не возрастает.
		if i > 0 && item.CreatedAt.After(page.Items[i-1].CreatedAt) {
			t.Fatalf("порядок сессий не «новые первыми»: [%d]=%v > [%d]=%v",
				i, item.CreatedAt, i-1, page.Items[i-1].CreatedAt)
		}
	}
	// Более поздний вход — первая сессия в списке (при различных createdAt).
	if page.Items[0].CreatedAt.Before(page.Items[1].CreatedAt) {
		t.Fatalf("первая сессия старше второй: %v < %v", page.Items[0].CreatedAt, page.Items[1].CreatedAt)
	}

	// До DELETE все три cookie (две целевые и чужая) рабочие.
	for name, cookies := range map[string][]*http.Cookie{
		"target #1": targetCookies, "target #2": targetCookies2, "other": otherCookies,
	} {
		if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, cookies); rec.Code != http.StatusOK {
			t.Fatalf("%s: me до DELETE status=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}

	// DELETE: 204 без тела.
	rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+targetID+"/sessions", nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE сессий: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 не должен содержать тело: %s", rec.Body.String())
	}

	// Обе cookie целевого пользователя перестают проходить requireAuth.
	for i, cookies := range [][]*http.Cookie{targetCookies, targetCookies2} {
		if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, cookies); rec.Code != http.StatusUnauthorized {
			t.Fatalf("cookie #%d: me после DELETE status=%d, ждали 401: %s", i+1, rec.Code, rec.Body.String())
		}
		if rec := doRequest(t, router, http.MethodGet, "/api/v1/me/courses", nil, cookies); rec.Code != http.StatusUnauthorized {
			t.Fatalf("cookie #%d: me/courses после DELETE status=%d, ждали 401", i+1, rec.Code)
		}
	}

	// Список пуст (но items — массив).
	page, raw = t93GetSessions(t, router, adminCookies, targetID)
	if len(page.Items) != 0 || len(raw) != 0 {
		t.Fatalf("после DELETE остались сессии: %+v", page.Items)
	}

	// Чужие сессии не тронуты: ученик и сам админ продолжают работать.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, otherCookies); rec.Code != http.StatusOK {
		t.Fatalf("чужая сессия ученика после DELETE: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, adminCookies); rec.Code != http.StatusOK {
		t.Fatalf("сессия админа после DELETE: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := app.Store.ListSessionsByUser(otherID); len(got) != 1 {
		t.Fatalf("у чужого ученика %d сессий, ждали 1", len(got))
	}

	// DELETE удалил и уже истёкшую запись: повторный вызов на уровне store
	// возвращает 0, значит чистить больше нечего.
	if n := app.Store.DeleteSessionsByUser(targetID); n != 0 {
		t.Fatalf("после DELETE осталось сессий: %d", n)
	}

	// Повторный DELETE идемпотентен — 204.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+targetID+"/sessions", nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("повторный DELETE: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// После принудительного выхода пользователь может войти снова, и новая
	// сессия снова видна админу.
	freshCookies := loginUser(t, router, "sess-target@example.com", "password123")
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, freshCookies); rec.Code != http.StatusOK {
		t.Fatalf("me после повторного входа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	page, _ = t93GetSessions(t, router, adminCookies, targetID)
	if len(page.Items) != 1 {
		t.Fatalf("после повторного входа сессий %d, ждали 1", len(page.Items))
	}
}

// TestAdminUserSessionsGuardsAndNotFound — 401 без входа, 403 не-админу,
// 404 для несуществующего/некорректного id; гварды не удаляют чужие сессии.
func TestAdminUserSessionsGuardsAndNotFound(t *testing.T) {
	app, router := newTestApp()
	adminCookies, _ := seedAdminUser(t, app, router, "sess-guard-admin@example.com")
	studentCookies, studentID := registerUser(t, router, "sess-guard-student@example.com", "password123", "")

	path := "/api/v1/admin/users/" + studentID + "/sessions"
	missing := "/api/v1/admin/users/00000000-0000-0000-0000-000000000000/sessions"

	// 401: аноним не может ни читать, ни удалять.
	if rec := doRequest(t, router, http.MethodGet, path, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним GET: status=%d, ждали 401", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodDelete, path, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("аноним DELETE: status=%d, ждали 401", rec.Code)
	}

	// 403: ученик не админ — даже для собственных сессий.
	if rec := doRequest(t, router, http.MethodGet, path, nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("ученик GET: status=%d, ждали 403", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodDelete, path, nil, studentCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("ученик DELETE: status=%d, ждали 403", rec.Code)
	}

	// Отказы 401/403 не удалили сессию ученика.
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, studentCookies); rec.Code != http.StatusOK {
		t.Fatalf("me ученика после отказов: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 404: несуществующий пользователь — в обоих методах.
	if rec := doRequest(t, router, http.MethodGet, missing, nil, adminCookies); rec.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего: status=%d, ждали 404: %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodDelete, missing, nil, adminCookies); rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE несуществующего: status=%d, ждали 404: %s", rec.Code, rec.Body.String())
	}

	// 404: некорректный id — тоже «пользователь не найден», без 500.
	bad := "/api/v1/admin/users/not-a-uuid/sessions"
	if rec := doRequest(t, router, http.MethodGet, bad, nil, adminCookies); rec.Code != http.StatusNotFound {
		t.Fatalf("GET некорректного id: status=%d, ждали 404: %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodDelete, bad, nil, adminCookies); rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE некорректного id: status=%d, ждали 404: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminUserSessionsRevokeOwn — админ может завершить собственные сессии:
// текущая cookie сразу перестаёт работать, новый вход создаёт новую сессию.
func TestAdminUserSessionsRevokeOwn(t *testing.T) {
	app, router := newTestApp()
	adminCookies, adminID := seedAdminUser(t, app, router, "sess-self@example.com")

	page, _ := t93GetSessions(t, router, adminCookies, adminID)
	if len(page.Items) != 1 {
		t.Fatalf("своих сессий %d, ждали 1", len(page.Items))
	}

	rec := doRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+adminID+"/sessions", nil, adminCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE своих сессий: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, adminCookies); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me после выхода: status=%d, ждали 401", rec.Code)
	}

	freshCookies := loginUser(t, router, "sess-self@example.com", "password123")
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, freshCookies); rec.Code != http.StatusOK {
		t.Fatalf("me после повторного входа: status=%d body=%s", rec.Code, rec.Body.String())
	}
	page, _ = t93GetSessions(t, router, freshCookies, adminID)
	if len(page.Items) != 1 {
		t.Fatalf("сессий после повторного входа %d, ждали 1", len(page.Items))
	}
}
