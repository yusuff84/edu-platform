package handler

// Регрессионные тесты исправлений аудита бэкенда (ноябрь 2026): захват
// неподтверждённой учётки через вход Яндексом, раздача /uploads без листинга,
// страница mock-оплаты, пагинация без переполнения, схемы ссылок уроков.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/repo"
)

// yandexLogin проходит start → callback и возвращает ответ callback.
func yandexLogin(t *testing.T, router http.Handler, code string) *httptest.ResponseRecorder {
	t.Helper()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st := stateCookieOf(t, rec)
	return doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code="+code, "", withState(st))
}

// Учётку с чужим (неподтверждённым) email заранее регистрирует злоумышленник.
// Вход владельца почты через Яндекс должен отнять у регистратора доступ:
// его сессии и пароль больше не работают, email помечен подтверждённым.
func TestYandexLoginClaimsUnverifiedAccount(t *testing.T) {
	fake := &fakeYandex{profile: auth.Profile{ExternalID: "77", Email: "victim@ya.ru"}}
	app, router := oauthRouter(t, fake)

	reg := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"victim@ya.ru","password":"attacker-pass"}`, nil)
	if reg.Code != http.StatusCreated {
		t.Fatalf("регистрация: %d %s", reg.Code, reg.Body.String())
	}
	attacker := sessionCookieOf(t, reg)

	cb := yandexLogin(t, router, "claim-1")
	if cb.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", cb.Code, cb.Body.String())
	}
	owner := sessionCookieOf(t, cb)

	if me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", withCookie(attacker)); me.Code != http.StatusUnauthorized {
		t.Fatalf("сессия регистратора должна быть завершена: %d", me.Code)
	}
	if login := doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"victim@ya.ru","password":"attacker-pass"}`, nil); login.Code != http.StatusUnauthorized {
		t.Fatalf("пароль регистратора должен перестать работать: %d", login.Code)
	}
	me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", withCookie(owner))
	if me.Code != http.StatusOK {
		t.Fatalf("сессия владельца почты: %d %s", me.Code, me.Body.String())
	}
	u, _ := app.Store.GetUserByEmail("victim@ya.ru")
	if u.EmailVerifiedAt == nil {
		t.Fatal("email после входа через Яндекс должен быть подтверждён")
	}
	// Имя из профиля пустое — email именем не подставляется (он публичен в Q&A).
	if u.DisplayName != "" {
		t.Fatalf("displayName = %q, ждали пустое", u.DisplayName)
	}

	// Подтверждённую учётку повторный вход не трогает: свой пароль владельца
	// и прежняя сессия продолжают работать.
	hash, _ := bcrypt.GenerateFromPassword([]byte("owner-pass-1"), bcrypt.MinCost)
	if !app.Store.SetUserPasswordHash(u.ID, string(hash)) {
		t.Fatal("SetUserPasswordHash")
	}
	if cb := yandexLogin(t, router, "claim-2"); cb.Code != http.StatusFound {
		t.Fatalf("повторный callback: %d", cb.Code)
	}
	if login := doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"victim@ya.ru","password":"owner-pass-1"}`, nil); login.Code != http.StatusOK {
		t.Fatalf("пароль владельца после повторного входа Яндексом: %d", login.Code)
	}
	// SetUserPasswordHash сессии не трогает, значит сессия владельца жива,
	// только если повторный вход Яндексом не сбросил их заново.
	if me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", withCookie(owner)); me.Code != http.StatusOK {
		t.Fatalf("повторный вход Яндексом не должен завершать сессии подтверждённой учётки: %d", me.Code)
	}
}

// Имя из профиля Яндекса приводится к правилам профиля: пробелы схлопываются,
// не длиннее 60 символов.
func TestYandexDisplayNameNormalized(t *testing.T) {
	long := ""
	for i := 0; i < 70; i++ {
		long += "я"
	}
	fake := &fakeYandex{profile: auth.Profile{ExternalID: "78", Email: "longname@ya.ru", DisplayName: "  " + long + "  "}}
	app, router := oauthRouter(t, fake)
	if cb := yandexLogin(t, router, "name-1"); cb.Code != http.StatusFound {
		t.Fatalf("callback: %d", cb.Code)
	}
	u, _ := app.Store.GetUserByEmail("longname@ya.ru")
	if n := len([]rune(u.DisplayName)); n != 60 {
		t.Fatalf("длина имени %d, ждали 60: %q", n, u.DisplayName)
	}
}

// /uploads раздаёт файлы, но не листинг каталогов, и ставит nosniff.
func TestUploadsHandlerNoDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "covers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "covers", "a.png"), []byte("\x89PNG\r\n\x1a\nxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := http.StripPrefix("/uploads/", uploadsHandler(http.Dir(dir)))
	for _, p := range []string{"/uploads/", "/uploads/covers/", "/uploads/covers", "/uploads/missing.png"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status=%d, ждали 404; body=%q", p, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/covers/a.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("файл: status=%d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("нет nosniff: %v", rec.Header())
	}
}

// Страница mock-оплаты не раскрывает платежи других провайдеров и в
// production не показывает ничего.
func TestMockPaymentPageOnlyMockPayments(t *testing.T) {
	app, router := newTestRouter(devConfig())
	u := mustUser(t, app, "mockpage@x.ru", repo.RoleStudent)
	c := app.Store.CreateCourse(&repo.Course{TeacherID: u.ID, Slug: "mock-page", Title: "Секретный курс", Description: "d", PriceCents: 12345})
	for _, p := range []*repo.Payment{
		{StudentID: u.ID, CourseID: c.ID, AmountCents: 12345, TeacherAmountCents: 12345, Currency: "RUB",
			Provider: "yookassa", ProviderPaymentID: "yk-real-1", Status: repo.PaymentPending},
		{StudentID: u.ID, CourseID: c.ID, AmountCents: 12345, TeacherAmountCents: 12345, Currency: "RUB",
			Provider: "mock", ProviderPaymentID: "mock-1", Status: repo.PaymentPending},
	} {
		if _, err := app.Store.CreatePayment(p); err != nil {
			t.Fatal(err)
		}
	}
	page := func(router http.Handler, id string) string {
		rec := doJSON(t, router, http.MethodGet, "/pay/mock/"+id, "", nil)
		return rec.Body.String()
	}
	if body := page(router, "yk-real-1"); !strings.Contains(body, "Платёж не найден") || strings.Contains(body, "Секретный курс") {
		t.Fatalf("платёж YooKassa не должен показываться: %s", body)
	}
	if body := page(router, "mock-1"); !strings.Contains(body, "Секретный курс") {
		t.Fatalf("mock-платёж в dev должен показываться: %s", body)
	}
	prodApp := &App{Store: app.Store, Config: prodConfig()}
	if body := page(NewRouter(prodApp), "mock-1"); strings.Contains(body, "Секретный курс") {
		t.Fatalf("в production страница mock-оплаты не должна раскрывать платёж: %s", body)
	}
}

// paginate не переполняет int на огромном page.
func TestPaginateHugePage(t *testing.T) {
	start, page, limit := paginate(5, 9223372036854775807, 20)
	if start != 5 || page != 9223372036854775807 || limit != 20 {
		t.Fatalf("paginate: start=%d page=%d limit=%d", start, page, limit)
	}
	if got := pageSlice([]int{1, 2, 3}, 9223372036854775807, 100); len(got) != 0 {
		t.Fatalf("pageSlice: %v", got)
	}
}

func TestValidateLessonLinks(t *testing.T) {
	cases := map[string]bool{
		"":                         true,
		"https://youtu.be/x":       true,
		"http://example.com/v.mp4": true,
		"/uploads/media/a.mp4":     true,
		"zoom.us/j/123":            true,
		"javascript:alert(1)":      false,
		" JavaScript:alert(1)":     false,
		"data:text/html,<b>x</b>":  false,
		"vbscript:msgbox":          false,
		"java\nscript:alert(1)":    false,
	}
	for link, ok := range cases {
		got := validateLessonLinks(&repo.Lesson{LiveURL: link}) == ""
		if got != ok {
			t.Errorf("liveUrl %q: валиден=%v, ждали %v", link, got, ok)
		}
		got = validateLessonLinks(&repo.Lesson{VideoURL: link}) == ""
		if got != ok {
			t.Errorf("videoUrl %q: валиден=%v, ждали %v", link, got, ok)
		}
	}
}
