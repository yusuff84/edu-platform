package handler

// T41: тесты транслитерации кириллицы в slug и уникализации авто-slug.
// Таблица примеров — из постановки Task/T41-slug-translit.md и контракта
// (Task/00-api-contract.md, «Slug: транслитерация кириллицы (T41)»).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"edu-platform/backend/internal/repo"
)

// TestTranslitSlug — таблица транслитерации: кириллица, мусор, дефисы, регистр.
func TestTranslitSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Мой Курс", "moy-kurs"},
		{"Ёжик и Яблоки", "ezhik-i-yabloki"},
		{"Тест №1!", "test-1"},
		{"---", ""},
		{"", ""},
		{"Python 3.10", "python-3-10"},
		{"  Мой   Курс  ", "moy-kurs"},
		{"Мой--Курс", "moy-kurs"},
		{"Объявление", "obyavlenie"}, // ъ не даёт символа
		{"ь", ""},                    // ь не даёт символа
		{"ЖЮЩШЧЦ", "zhyuschshchc"},   // составные: ж→zh, ю→yu, щ→sch, ш→sh, ч→ch
		{"Цель — 100%", "cel-100"},   // длинное тире и % — дефисы, крайние срезаны
	}
	for _, tc := range cases {
		if got := translitSlug(tc.in); got != tc.want {
			t.Errorf("translitSlug(%q) = %q, ждали %q", tc.in, got, tc.want)
		}
	}

	// Обрезка до 120 символов; дефис на границе обрезки не остаётся.
	long120 := strings.Repeat("а", 120)
	long121 := strings.Repeat("а", 121)
	if got := translitSlug(long120); got != strings.Repeat("a", 120) {
		t.Errorf("120 символов не должны обрезаться: len=%d", len(got))
	}
	if got := translitSlug(long121); len(got) != 120 || got != strings.Repeat("a", 120) {
		t.Errorf("строка длиннее 120 не обрезана: len=%d", len(got))
	}
	// 119 «a» + «-b» даёт 121 символ: обрезка попадает на дефис — он снимается.
	if got := translitSlug(strings.Repeat("a", 119) + "-b"); got != strings.Repeat("a", 119) {
		t.Errorf("дефис на границе обрезки не срезан: len=%d, хвост=%q", len(got), got[len(got)-3:])
	}
}

// TestSlugFromTitle — короткая/пустая основа обозначается пустой строкой
// (fallback на course-<hex8> решает makeSlug).
func TestSlugFromTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Мой Курс", "moy-kurs"},
		{"Go-1", "go-1"},
		{"Go", ""},  // 2 символа — короче лимита slugRe
		{"№1", ""},  // после транслитерации осталось «1»
		{"!!!", ""}, // не осталось ничего
	}
	for _, tc := range cases {
		if got := slugFromTitle(tc.in); got != tc.want {
			t.Errorf("slugFromTitle(%q) = %q, ждали %q", tc.in, got, tc.want)
		}
	}
}

// TestSlugWithSuffix — суффикс -2…-9 укладывается в лимит 120 символов.
func TestSlugWithSuffix(t *testing.T) {
	if got := slugWithSuffix("moy-kurs", 2); got != "moy-kurs-2" {
		t.Errorf("короткая основа: %q, ждали moy-kurs-2", got)
	}
	full := strings.Repeat("a", 120)
	got := slugWithSuffix(full, 2)
	if len(got) != 120 || got != strings.Repeat("a", 118)+"-2" {
		t.Errorf("длинная основа: len=%d, хвост=%q", len(got), got[len(got)-4:])
	}
	if !slugRe.MatchString(got) {
		t.Errorf("slug с суффиксом не проходит slugRe: %q", got)
	}
}

// TestMakeSlugTranslitAndUnique — POST /teacher/courses сквозь роутер:
// кастомный кириллический slug транслитерируется, пустой строится из названия,
// занятость разрешается суффиксами -2…-9, затем course-<hex8>.
func TestMakeSlugTranslitAndUnique(t *testing.T) {
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, "t-slug-translit@x.ru", repo.RoleTeacher)
	cookie := sessionCookieFor(t, app, u.ID)

	create := func(body string) (int, string) {
		t.Helper()
		rec := doJSON(t, router, http.MethodPost, "/api/v1/teacher/courses", body, withCookie(cookie))
		var c struct {
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &c)
		return rec.Code, c.Slug
	}

	// Кастомный slug «Мой курс» → 201 и slug «moy-kurs» (ручная проверка T41).
	if code, slug := create(`{"title":"Курс","description":"О","slug":"Мой курс"}`); code != http.StatusCreated || slug != "moy-kurs" {
		t.Fatalf("кастомный кириллический slug: status=%d slug=%q, ждали 201/moy-kurs", code, slug)
	}

	// Пустой slug → основа из названия; «moy-kurs» занят → суффикс -2.
	if code, slug := create(`{"title":"Мой курс","description":"О"}`); code != http.StatusCreated || slug != "moy-kurs-2" {
		t.Fatalf("авто-slug с занятой основой: status=%d slug=%q, ждали 201/moy-kurs-2", code, slug)
	}

	// Занимаем -3…-9 — все суффиксы исчерпаны, значит course-<hex8>.
	for i := 3; i <= 9; i++ {
		app.Store.CreateCourse(&repo.Course{
			TeacherID: u.ID, Slug: fmt.Sprintf("moy-kurs-%d", i), Title: "x", Description: "y",
		})
	}
	hex8 := regexp.MustCompile(`^course-[0-9a-f]{8}$`)
	if code, slug := create(`{"title":"Мой курс","description":"О"}`); code != http.StatusCreated || !hex8.MatchString(slug) {
		t.Fatalf("исчерпание суффиксов: status=%d slug=%q, ждали 201/course-xxxxxxxx", code, slug)
	}

	// Короткая основа («№1» → «1») — сразу course-<hex8>.
	if code, slug := create(`{"title":"№1","description":"О"}`); code != http.StatusCreated || !hex8.MatchString(slug) {
		t.Fatalf("короткая основа: status=%d slug=%q, ждали 201/course-xxxxxxxx", code, slug)
	}

	// Мусор, от которого после транслитерации ничего не осталось, — 400.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/teacher/courses",
		`{"title":"Курс","description":"О","slug":"!!!"}`, withCookie(cookie))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("невалидный после транслитерации slug: status=%d, ждали 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Некорректный или занятый slug") {
		t.Fatalf("текст ошибки изменён: %s", rec.Body.String())
	}
}

// TestUpdateCourseTranslitSlug — PATCH /teacher/courses/{id}: кириллица
// транслитерируется; проверки занятости и тексты ошибок прежние.
func TestUpdateCourseTranslitSlug(t *testing.T) {
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, "t-slug-patch@x.ru", repo.RoleTeacher)
	cookie := sessionCookieFor(t, app, u.ID)
	c := app.Store.CreateCourse(&repo.Course{
		TeacherID: u.ID, Slug: "start-slug", Title: "Тест", Description: "О",
	})

	// «Мой Курс» → «moy-kurs»: 200 и сохранённый slug.
	rec := doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"slug":"Мой Курс"}`, withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH кириллицей: status=%d body=%s, ждали 200", rec.Code, rec.Body.String())
	}
	var updated repo.Course
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if updated.Slug != "moy-kurs" {
		t.Fatalf("slug после транслитерации: %q, ждали moy-kurs", updated.Slug)
	}

	// Занятый ПОСЛЕ транслитерации slug — прежний 400 и прежний текст.
	app.Store.CreateCourse(&repo.Course{TeacherID: u.ID, Slug: "zanyat", Title: "x", Description: "y"})
	rec = doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"slug":"Занят"}`, withCookie(cookie))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH занятым slug: status=%d, ждали 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Такой slug уже занят") {
		t.Fatalf("текст ошибки занятости изменён: %s", rec.Body.String())
	}

	// Мусор после транслитерации — 400 с прежним текстом.
	rec = doJSON(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+c.ID,
		`{"slug":"!!!"}`, withCookie(cookie))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH невалидным slug: status=%d, ждали 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Некорректный slug") {
		t.Fatalf("текст ошибки валидации изменён: %s", rec.Body.String())
	}
}
