package contract_test

// T33: правка карточки курса PATCH /teacher/courses/{id} — статусная матрица.
//
// Контракт (Task/00-api-contract.md, «Правка карточки курса (T33)»):
//   - draft | review | published → 200: осознанное исключение из правила
//     «менять можно только черновик» — карточка не проходит модерацию
//     заново, правка опубликованного курса его не сбрасывает;
//   - archived → 409 «Курс в архиве: карточка заморожена»; сначала
//     POST /teacher/courses/{id}/unarchive.
//
// Guard статуса стоит раньше валидации тела: пустой PATCH по архиву — 409,
// а не 400. Админ правит чужой курс тем же маршрутом — guard действует и
// для него. Валидация (занятый slug, пустое название) для неархивных
// статусов — по-прежнему 400 (регрессия).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestTeacherCardPatchStatusMatrix — обязательные случаи T33 через роутер:
// published/review/draft → 200, archived → 409 без изменения стора,
// archived+пустое тело → 409, админ на чужом архиве → 409, занятый slug и
// пустое название → 400.
func TestTeacherCardPatchStatusMatrix(t *testing.T) {
	app, router := newTestApp()
	cookies, courseID, _ := seedPublishedCourse(t, app, router, 1000)
	path := "/api/v1/teacher/courses/" + courseID

	patch := func(who []*http.Cookie, body any) *httptest.ResponseRecorder {
		t.Helper()
		return doRequest(t, router, http.MethodPatch, path, body, who)
	}
	var card map[string]any

	// 1. published → 200: поля обновились, статус остался published.
	rec := patch(cookies, map[string]any{"title": "Новое название", "priceCents": 2500})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH в published: status=%d body=%s, ждали 200", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &card)
	if card["title"] != "Новое название" {
		t.Fatalf("title не обновился: %v", card["title"])
	}
	if price, _ := card["priceCents"].(float64); price != 2500 {
		t.Fatalf("priceCents не обновился: %v", card["priceCents"])
	}
	if card["status"] != "published" {
		t.Fatalf("правка не должна менять статус: %v", card["status"])
	}

	// 2. Архивация: published → archived.
	rec = doRequest(t, router, http.MethodPost, path+"/archive", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 3. archived → 409, курс в сторе не изменился (title/priceCents/статус).
	rec = patch(cookies, map[string]any{"title": "Правка архива", "priceCents": 9999})
	if rec.Code != http.StatusConflict {
		t.Fatalf("PATCH в archived: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "заморожена") {
		t.Fatalf("ожидали текст «карточка заморожена»: %s", rec.Body.String())
	}
	stored, ok := app.Store.GetCourseByID(courseID)
	if !ok {
		t.Fatal("курс пропал из стора")
	}
	if stored.Title != "Новое название" || stored.PriceCents != 2500 || stored.Status != repo.CourseArchived {
		t.Fatalf("архивный курс изменился: %+v", stored)
	}

	// 4. archived + пустое тело → 409: состояние ресурса раньше валидации.
	rec = patch(cookies, map[string]any{})
	if rec.Code != http.StatusConflict {
		t.Fatalf("пустой PATCH в archived: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}

	// 5. Админ правит чужой архивный курс — тот же guard, 409.
	adminCookies, _ := registerUser(t, router, "card-admin@example.com", "password123", "")
	promoteToAdmin(t, app, "card-admin@example.com")
	rec = patch(adminCookies, map[string]any{"title": "Админ правит архив"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("PATCH архива админом: status=%d body=%s, ждали 409", rec.Code, rec.Body.String())
	}

	// 6. unarchive → draft, правка снова работает.
	rec = doRequest(t, router, http.MethodPost, path+"/unarchive", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("unarchive: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = patch(cookies, map[string]any{"title": "После архива", "priceCents": 3100})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH после unarchive: status=%d body=%s, ждали 200", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &card)
	if card["title"] != "После архива" {
		t.Fatalf("title не обновился после unarchive: %v", card["title"])
	}
	if price, _ := card["priceCents"].(float64); price != 3100 {
		t.Fatalf("priceCents не обновился после unarchive: %v", card["priceCents"])
	}
	if card["status"] != "draft" {
		t.Fatalf("статус после unarchive: %v", card["status"])
	}

	// 7. Валидация не сломана: занятый slug и пустое название — 400.
	rec = doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Занятый slug", "description": "Описание", "priceCents": 0, "slug": "busy-card-slug",
	}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("создание второго курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = patch(cookies, map[string]any{"slug": "busy-card-slug"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("занятый slug: status=%d body=%s, ждали 400", rec.Code, rec.Body.String())
	}
	rec = patch(cookies, map[string]any{"title": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустое название: status=%d body=%s, ждали 400", rec.Code, rec.Body.String())
	}

	// 8. review → 200: карточка на модерации тоже правится.
	rec = doRequest(t, router, http.MethodPost, path+"/submit", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = patch(cookies, map[string]any{"title": "На модерации"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH в review: status=%d body=%s, ждали 200", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &card)
	if card["title"] != "На модерации" {
		t.Fatalf("title не обновился в review: %v", card["title"])
	}
	if card["status"] != "review" {
		t.Fatalf("правка не должна сбрасывать review: %v", card["status"])
	}
}
