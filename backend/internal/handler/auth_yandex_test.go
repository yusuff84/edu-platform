package handler

// Тесты маршрутов Яндекс OAuth (T13): старт-редирект, сверка state,
// find-or-create пользователя и выдача сессии. Провайдер подменяется
// через App.YandexAuth — сеть не трогается.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/config"
	"edu-platform/backend/internal/repo"
)

// fakeYandex — тестовый AuthProvider: AuthURL отдаёт фиктивный адрес,
// HandleCallback — профиль из полей.
type fakeYandex struct {
	profile  auth.Profile
	err      error
	lastCode string
}

func (f *fakeYandex) AuthURL(state string) (string, error) {
	return "http://yandex.invalid/authorize?state=" + url.QueryEscape(state), nil
}

func (f *fakeYandex) HandleCallback(_ context.Context, code string) (auth.Profile, error) {
	f.lastCode = code
	if f.err != nil {
		return auth.Profile{}, f.err
	}
	return f.profile, nil
}

// oauthRouter поднимает роутер с тестовым провайдером Яндекса.
func oauthRouter(t *testing.T, fake *fakeYandex) (*App, http.Handler) {
	t.Helper()
	cfg := &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		RunnerURL:        "http://127.0.0.1:1",
		AllowedOrigins:   []string{"http://localhost:3000"},
		YandexClientID:   "тестовый-id",
	}
	app := &App{Store: repo.NewStore(), Config: cfg, YandexAuth: fake}
	return app, NewRouter(app)
}

// withState — отправка callback с cookie состояния (как делает браузер
// при редиректе обратно на наш домен).
func withState(st *http.Cookie) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(st) }
}

func TestYandexAuthStartRedirectsAndSetsState(t *testing.T) {
	_, router := oauthRouter(t, &fakeYandex{})

	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("start: status=%d, ждали 302; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "http://yandex.invalid/authorize") {
		t.Fatalf("редирект не на провайдера: %q", loc)
	}

	// state в URL обязан совпадать с cookie.
	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == oauthStateCookie {
			stateCookie = c
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatalf("нет oauth_state cookie; Set-Cookie: %v", rec.Header().Values("Set-Cookie"))
	}
	if !strings.Contains(loc, "state="+url.QueryEscape(stateCookie.Value)) {
		t.Fatalf("state в URL не совпадает с cookie: url=%q cookie=%q", loc, stateCookie.Value)
	}
	if !stateCookie.HttpOnly {
		t.Fatal("oauth_state должен быть httpOnly")
	}
}

func TestYandexAuthStartWithoutClientID(t *testing.T) {
	cfg := devConfig() // YandexClientID пуст
	app := &App{Store: repo.NewStore(), Config: cfg, YandexAuth: &auth.YandexClient{}}
	router := NewRouter(app)
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("start без ключей: status=%d, ждали 503; body=%s", rec.Code, rec.Body.String())
	}
}

// stateCookieOf достаёт oauth_state cookie из ответа start.
func stateCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == oauthStateCookie {
			return c
		}
	}
	t.Fatalf("нет oauth_state cookie; Set-Cookie: %v", rec.Header().Values("Set-Cookie"))
	return nil
}

func TestYandexAuthCallbackFullFlow(t *testing.T) {
	fake := &fakeYandex{profile: auth.Profile{
		ExternalID: "42", Email: "ivan@ya.ru", DisplayName: "Иван",
	}}
	app, router := oauthRouter(t, fake)

	// 1. Старт — забираем state-cookie.
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st := stateCookieOf(t, rec)

	// 2. Callback с корректным state и code (cookie отправляем, как браузер) —
	// пользователь создан, сессия выдана.
	cb := doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code=oauth-code-1", "", withState(st))
	if cb.Code != http.StatusFound {
		t.Fatalf("callback: status=%d, ждали 302; body=%s", cb.Code, cb.Body.String())
	}
	if fake.lastCode != "oauth-code-1" {
		t.Fatalf("код не дошёл до провайдера: %q", fake.lastCode)
	}
	var sess *http.Cookie
	for _, c := range cb.Result().Cookies() {
		if c.Name == sessionCookie {
			sess = c
		}
	}
	if sess == nil {
		t.Fatal("после callback не выдана session cookie")
	}
	u, ok := app.Store.GetUserByEmail("ivan@ya.ru")
	if !ok {
		t.Fatal("пользователь не создан")
	}
	if u.Role != repo.RoleStudent {
		t.Fatalf("роль OAuth-пользователя: %s, ждали student", u.Role)
	}
	if u.DisplayName != "Иван" {
		t.Fatalf("displayName не сохранён из профиля: %q", u.DisplayName)
	}

	// 3. /auth/me с выданной cookie — 200.
	me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", "", withCookie(sess))
	if me.Code != http.StatusOK {
		t.Fatalf("me: status=%d body=%s", me.Code, me.Body.String())
	}

	// 4. Повторный вход того же email — новый пользователь НЕ создаётся.
	before, _ := app.Store.GetUserByEmail("ivan@ya.ru")
	rec = doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st = stateCookieOf(t, rec)
	cb = doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code=oauth-code-2", "", withState(st))
	if cb.Code != http.StatusFound {
		t.Fatalf("повторный callback: status=%d body=%s", cb.Code, cb.Body.String())
	}
	after, _ := app.Store.GetUserByEmail("ivan@ya.ru")
	if before.ID != after.ID {
		t.Fatal("повторный вход должен найти существующего пользователя")
	}
}

func TestYandexAuthCallbackRejectsBadState(t *testing.T) {
	fake := &fakeYandex{profile: auth.Profile{ExternalID: "1", Email: "x@ya.ru"}}
	_, router := oauthRouter(t, fake)

	// 1. Неверный state (cookie есть, query не совпадает) — 400,
	// провайдер не вызывается.
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st := stateCookieOf(t, rec)
	rec = doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state=чужой&code=c", "", withState(st))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("чужой state: status=%d, ждали 400", rec.Code)
	}
	if fake.lastCode != "" {
		t.Fatalf("провайдер не должен вызываться при неверном state, code=%q", fake.lastCode)
	}

	// 2. Вовсе без state-cookie — 400.
	rec = doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state=любой&code=c", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("без cookie: status=%d, ждали 400", rec.Code)
	}

	// 3. Без code — 400.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st = stateCookieOf(t, rec)
	rec = doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value), "", withState(st))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("без code: status=%d, ждали 400", rec.Code)
	}
}

func TestYandexAuthCallbackOAuthError(t *testing.T) {
	fake := &fakeYandex{err: &auth.OAuthError{Op: "обмен кода на токен", Msg: "код авторизации недействителен или просрочен"}}
	_, router := oauthRouter(t, fake)

	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st := stateCookieOf(t, rec)
	cb := doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code=bad", "", withState(st))
	if cb.Code != http.StatusBadGateway {
		t.Fatalf("ошибка провайдера: status=%d, ждали 502; body=%s", cb.Code, cb.Body.String())
	}
	if !strings.Contains(cb.Body.String(), "недействителен") {
		t.Fatalf("русский текст ошибки провайдера не дошёл: %s", cb.Body.String())
	}
}

// TestYandexFullFlowWithHTTPTest — оба хендлера через НАСТОЯЩИЙ YandexClient,
// чьи TokenURL/InfoURL подменены на httptest.Server (поля экспортированы T10
// именно для этого). Сеть не трогается: проверяется полный путь
// start → callback → обмен code на токен → профиль → сессия.
func TestYandexFullFlowWithHTTPTest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "живой-код" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"токен","token_type":"bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth токен" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":777,"login":"petr","display_name":"Пётр","default_email":"petr@ya.ru"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &config.Config{
		PaymentsProvider: "mock",
		CommissionPct:    15,
		RunnerURL:        "http://127.0.0.1:1",
		AllowedOrigins:   []string{"http://localhost:3000"},
		YandexClientID:   "тестовый-id",
	}
	app := &App{Store: repo.NewStore(), Config: cfg, YandexAuth: &auth.YandexClient{
		ClientID:     "тестовый-id",
		ClientSecret: "тестовый-секрет",
		RedirectURI:  "http://localhost:8082/api/v1/auth/yandex/callback",
		AuthURLBase:  ts.URL + "/authorize",
		TokenURL:     ts.URL + "/token",
		InfoURL:      ts.URL + "/info",
	}}
	router := NewRouter(app)

	// 1. Start — 302 на фейковый authorize, state в cookie и URL.
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("start: status=%d, ждали 302; body=%s", rec.Code, rec.Body.String())
	}
	st := stateCookieOf(t, rec)
	if !strings.Contains(rec.Header().Get("Location"), "state="+url.QueryEscape(st.Value)) {
		t.Fatalf("state не дошёл до authorize: loc=%q cookie=%v", rec.Header().Get("Location"), st)
	}

	// 2. Callback с корректным code (отправляем state-cookie, как браузер) —
	// реальный обмен через httptest.
	cb := doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code="+url.QueryEscape("живой-код"), "", withState(st))
	if cb.Code != http.StatusFound {
		t.Fatalf("callback: status=%d, ждали 302; body=%s", cb.Code, cb.Body.String())
	}
	u, ok := app.Store.GetUserByEmail("petr@ya.ru")
	if !ok {
		t.Fatalf("пользователь не создан по профилю: %v", cb.Body.String())
	}
	if u.DisplayName != "Пётр" {
		t.Fatalf("displayName из info: %q, ждали «Пётр»", u.DisplayName)
	}
	// Cookie state очищен (MaxAge <= 0).
	for _, c := range cb.Result().Cookies() {
		if c.Name == oauthStateCookie && c.MaxAge > 0 {
			t.Fatalf("state-cookie должна быть очищена после callback: %+v", c)
		}
	}

	// 3. Браузер очистил cookie (MaxAge=-1) — повторный callback без cookie → 400.
	again := doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code=x", "", nil)
	if again.Code != http.StatusBadRequest {
		t.Fatalf("повторный callback: status=%d, ждали 400", again.Code)
	}
}

// TestYandexFullFlowEmaillessProfile — профиль без email: пользователь
// ключуется по ExternalID (синтетический email), вход всё равно работает.
func TestYandexFullFlowEmaillessProfile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"токен","token_type":"bearer"}`))
	})
	mux.HandleFunc("/info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"9001","display_name":"Без Почты"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	app := &App{Store: repo.NewStore(), Config: devConfig(), YandexAuth: &auth.YandexClient{
		ClientID: "id", ClientSecret: "секрет",
		TokenURL: ts.URL + "/token", InfoURL: ts.URL + "/info",
	}}
	router := NewRouter(app)

	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/yandex/start", "", nil)
	st := stateCookieOf(t, rec)
	cb := doJSON(t, router, http.MethodGet,
		"/api/v1/auth/yandex/callback?state="+url.QueryEscape(st.Value)+"&code=c", "", withState(st))
	if cb.Code != http.StatusFound {
		t.Fatalf("callback без email: status=%d body=%s", cb.Code, cb.Body.String())
	}
	if _, ok := app.Store.GetUserByEmail("yandex-9001@oauth.invalid"); !ok {
		t.Fatal("пользователь без email должен ключеваться по ExternalID")
	}
}
