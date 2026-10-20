package contract_test

// T73 — контрактные тесты избранных курсов:
//   PUT/DELETE /me/favorites/{courseId} — идемпотентные 204;
//   GET /me/favorites — 200 {"items":[CourseItem],"total":N}, новые первыми;
//   только published-курсы; чужое избранное не видно; без входа — 401.
// Контракт заморожен в Task/00-api-contract.md (раздел «T73 — Избранное»).

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// t73Teacher создаёт преподавателя напрямую в хранилище: курсы для тестов
// избранного не обязаны проходить модерацию, достаточно статуса published.
func t73Teacher(t *testing.T, app *handler.App) *repo.User {
	t.Helper()
	u, err := app.Store.CreateUser("t73-teacher@example.test", "hash", repo.RoleTeacher)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

// t73Course создаёт курс преподавателя и переводит его в нужный статус.
func t73Course(t *testing.T, app *handler.App, teacherID, slug, title string, status repo.CourseStatus) string {
	t.Helper()
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID:   teacherID,
		Slug:        slug,
		Title:       title,
		Description: "Курс для тестов избранного T73",
		PriceCents:  190000,
		Currency:    "RUB",
	})
	if c.ID == "" {
		t.Fatalf("CreateCourse(%s) не вернул id", slug)
	}
	if status != repo.CourseDraft && !app.Store.SetCourseStatus(c.ID, status, "") {
		t.Fatalf("SetCourseStatus(%s, %s) не сработал", c.ID, status)
	}
	return c.ID
}

// t73Favorites читает GET /me/favorites и проверяет форму конверта:
// items — массив (не null), total — число.
func t73Favorites(t *testing.T, router http.Handler, cookies []*http.Cookie) ([]map[string]any, map[string]any, string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/favorites", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /me/favorites: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var out map[string]any
	decodeBody(t, rec, &out)
	raw, ok := out["items"].([]any)
	if !ok {
		t.Fatalf("items должен быть массивом (не null): %s", body)
	}
	items := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		item, ok := it.(map[string]any)
		if !ok {
			t.Fatalf("элемент items не объект: %v (%s)", it, body)
		}
		items = append(items, item)
	}
	if total, ok := out["total"].(float64); !ok || int(total) != len(items) {
		t.Fatalf("total=%v, ожидали %d: %s", out["total"], len(items), body)
	}
	return items, out, body
}

// t73FavoriteIDs — id курсов избранного в порядке ответа.
func t73FavoriteIDs(items []map[string]any) []string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		id, _ := it["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// TestFavoritesRequireAuth: все три маршрута без входа отвечают 401.
func TestFavoritesRequireAuth(t *testing.T) {
	_, router := newTestApp()
	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/me/favorites"},
		{http.MethodPut, "/api/v1/me/favorites/00000000-0000-0000-0000-000000000001"},
		{http.MethodDelete, "/api/v1/me/favorites/00000000-0000-0000-0000-000000000001"},
	}
	for _, tc := range cases {
		rec := doRequest(t, router, tc.method, tc.path, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s без входа: status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TestFavoritesAddIsIdempotent: добавление и повторное добавление — 204,
// в списке одна запись; форма элемента — CourseItem без признака isFavorite.
func TestFavoritesAddIsIdempotent(t *testing.T) {
	app, router := newTestApp()
	teacher := t73Teacher(t, app)
	courseID := t73Course(t, app, teacher.ID, "t73-add", "Избранный курс", repo.CoursePublished)
	cookies, _ := registerUser(t, router, "t73-add@example.com", "password123", "")

	for i := 0; i < 2; i++ {
		rec := doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+courseID, nil, cookies)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("PUT #%d: status=%d body=%s", i+1, rec.Code, rec.Body.String())
		}
		if strings.TrimSpace(rec.Body.String()) != "" {
			t.Fatalf("204 не должен содержать тело: %q", rec.Body.String())
		}
	}

	items, _, body := t73Favorites(t, router, cookies)
	if len(items) != 1 {
		t.Fatalf("избранного: %d, ожидали 1: %s", len(items), body)
	}
	item := items[0]
	if item["id"] != courseID {
		t.Fatalf("id=%v, ожидали %s: %s", item["id"], courseID, body)
	}
	if item["title"] != "Избранный курс" || item["slug"] != "t73-add" {
		t.Fatalf("элемент не про запрошенный курс: %s", body)
	}
	if item["priceCents"] != float64(190000) || item["currency"] != "RUB" {
		t.Fatalf("цена/валюта элемента: %s", body)
	}
	if item["category"] != "other" {
		t.Fatalf("category=%v, ожидали other: %s", item["category"], body)
	}
	if _, ok := item["isFavorite"]; ok {
		t.Fatalf("признака isFavorite быть не должно: %s", body)
	}
}

// TestFavoritesDeleteIsIdempotent: удаление и повторное удаление — 204,
// список пуст и отдаётся массивом (не null); несуществующий курс — тоже 204.
func TestFavoritesDeleteIsIdempotent(t *testing.T) {
	app, router := newTestApp()
	teacher := t73Teacher(t, app)
	courseID := t73Course(t, app, teacher.ID, "t73-del", "Курс на удаление", repo.CoursePublished)
	cookies, _ := registerUser(t, router, "t73-del@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+courseID, nil, cookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT: status=%d body=%s", rec.Code, rec.Body.String())
	}
	for i := 0; i < 2; i++ {
		rec = doRequest(t, router, http.MethodDelete, "/api/v1/me/favorites/"+courseID, nil, cookies)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE #%d: status=%d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}
	// Курс, которого никогда не было в избранном, — идемпотентный 204.
	rec = doRequest(t, router, http.MethodDelete,
		"/api/v1/me/favorites/00000000-0000-0000-0000-000000000001", nil, cookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE неизвестного курса: status=%d body=%s", rec.Code, rec.Body.String())
	}

	items, _, body := t73Favorites(t, router, cookies)
	if len(items) != 0 {
		t.Fatalf("избранное должно быть пустым: %s", body)
	}
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("пустой список должен быть [], а не null: %s", body)
	}
}

// TestFavoritesListOrderNewestFirst: добавленные позже идут первыми.
func TestFavoritesListOrderNewestFirst(t *testing.T) {
	app, router := newTestApp()
	teacher := t73Teacher(t, app)
	first := t73Course(t, app, teacher.ID, "t73-first", "Первый", repo.CoursePublished)
	second := t73Course(t, app, teacher.ID, "t73-second", "Второй", repo.CoursePublished)
	third := t73Course(t, app, teacher.ID, "t73-third", "Третий", repo.CoursePublished)
	cookies, _ := registerUser(t, router, "t73-order@example.com", "password123", "")

	for _, id := range []string{first, second, third} {
		rec := doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+id, nil, cookies)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("PUT %s: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		// Разносим created_at: контракт требует строгий порядок добавления.
		time.Sleep(2 * time.Millisecond)
	}

	items, _, body := t73Favorites(t, router, cookies)
	got := t73FavoriteIDs(items)
	want := []string{third, second, first}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("порядок избранного: %v, ожидали %v: %s", got, want, body)
	}
}

// TestFavoritesOnlyPublished: черновик/архив/модерация — 404 «Курс не найден»,
// в избранное не попадают; курс, отправленный в архив после добавления,
// исчезает из списка.
func TestFavoritesOnlyPublished(t *testing.T) {
	app, router := newTestApp()
	teacher := t73Teacher(t, app)
	draft := t73Course(t, app, teacher.ID, "t73-draft", "Черновик", repo.CourseDraft)
	archived := t73Course(t, app, teacher.ID, "t73-archived", "Архив", repo.CoursePublished)
	if !app.Store.SetCourseStatus(archived, repo.CourseArchived, "") {
		t.Fatal("не удалось перевести курс в архив")
	}
	review := t73Course(t, app, teacher.ID, "t73-review", "На модерации", repo.CourseReview)
	published := t73Course(t, app, teacher.ID, "t73-published", "Опубликован", repo.CoursePublished)
	cookies, _ := registerUser(t, router, "t73-status@example.com", "password123", "")

	for _, id := range []string{draft, archived, review, "00000000-0000-0000-0000-000000000001"} {
		rec := doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+id, nil, cookies)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("PUT неопубликованного %s: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		var errOut map[string]string
		decodeBody(t, rec, &errOut)
		if errOut["error"] != "Курс не найден" {
			t.Fatalf("текст 404: %q", errOut["error"])
		}
	}

	rec := doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+published, nil, cookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT published: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Черновик опубликовали — в избранном его нет: неудачный PUT не сохранился.
	if !app.Store.SetCourseStatus(draft, repo.CoursePublished, "") {
		t.Fatal("не удалось опубликовать черновик")
	}
	items, _, body := t73Favorites(t, router, cookies)
	if got := t73FavoriteIDs(items); len(got) != 1 || got[0] != published {
		t.Fatalf("избранное после публикации черновика: %v, ожидали только %s: %s", got, published, body)
	}

	// Публикация сменилась на архив — курс пропадает из списка.
	if !app.Store.SetCourseStatus(published, repo.CourseArchived, "") {
		t.Fatal("не удалось отправить курс в архив")
	}
	items, _, body = t73Favorites(t, router, cookies)
	if len(items) != 0 {
		t.Fatalf("архивный курс остался в избранном: %s", body)
	}
}

// TestFavoritesIsolatedPerUser: избранное одного ученика не видно другому
// и не пересекается при добавлении/удалении.
func TestFavoritesIsolatedPerUser(t *testing.T) {
	app, router := newTestApp()
	teacher := t73Teacher(t, app)
	courseID := t73Course(t, app, teacher.ID, "t73-shared", "Общий курс", repo.CoursePublished)

	alice, _ := registerUser(t, router, "t73-alice@example.com", "password123", "")
	bob, _ := registerUser(t, router, "t73-bob@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+courseID, nil, alice)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT alice: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if items, _, body := t73Favorites(t, router, alice); len(items) != 1 {
		t.Fatalf("избранное alice: %d, ожидали 1: %s", len(items), body)
	}
	if items, _, body := t73Favorites(t, router, bob); len(items) != 0 {
		t.Fatalf("чужое избранное видно bob: %s", body)
	}

	rec = doRequest(t, router, http.MethodPut, "/api/v1/me/favorites/"+courseID, nil, bob)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT bob: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/me/favorites/"+courseID, nil, bob)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE bob: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if items, _, body := t73Favorites(t, router, alice); len(items) != 1 {
		t.Fatalf("удаление bob затронуло alice: %s", body)
	}
}

// TestFavoritesJSONShape: сырой ответ содержит items/total и не содержит
// isFavorite; пустой конверт — ровно {"items":[],"total":0}.
func TestFavoritesEmptyResponseShape(t *testing.T) {
	_, router := newTestApp()
	cookies, _ := registerUser(t, router, "t73-empty@example.com", "password123", "")

	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/favorites", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	decodeBody(t, rec, &raw)
	if string(raw["items"]) != "[]" {
		t.Fatalf("items=%s, ожидали []", raw["items"])
	}
	if string(raw["total"]) != "0" {
		t.Fatalf("total=%s, ожидали 0", raw["total"])
	}
	if _, ok := raw["isFavorite"]; ok {
		t.Fatalf("в конверте не должно быть isFavorite: %s", rec.Body.String())
	}
}
