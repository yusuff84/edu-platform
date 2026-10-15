package auth

// Тесты клиента Яндекс OAuth (T10): обмен code на токен, получение профиля,
// AuthURL и русские ошибки без утечки внутренностей провайдера.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestYandex поднимает token/info серверы на httptest и собирает клиент.
func newTestYandex(t *testing.T, onToken, onInfo http.HandlerFunc) *YandexClient {
	t.Helper()
	tokenSrv := httptest.NewServer(onToken)
	t.Cleanup(tokenSrv.Close)
	infoSrv := httptest.NewServer(onInfo)
	t.Cleanup(infoSrv.Close)
	return &YandexClient{
		ClientID:     "cid",
		ClientSecret: "sec",
		RedirectURI:  "http://localhost:3000/callback",
		TokenURL:     tokenSrv.URL,
		InfoURL:      infoSrv.URL,
	}
}

func TestYandexExchangeCodeAndProfile(t *testing.T) {
	var gotAuthHeader string
	y := newTestYandex(t,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("token: метод %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("token: parse form: %v", err)
			}
			for field, want := range map[string]string{
				"grant_type":    "authorization_code",
				"code":          "code-123",
				"client_id":     "cid",
				"client_secret": "sec",
				"redirect_uri":  "http://localhost:3000/callback",
			} {
				if got := r.PostFormValue(field); got != want {
					t.Errorf("token: %s=%q, ждали %q", field, got, want)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok-123","token_type":"bearer","expires_in":15784388}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			gotAuthHeader = r.Header.Get("Authorization")
			if r.URL.Query().Get("format") != "json" {
				t.Errorf("info: нет format=json")
			}
			w.Header().Set("Content-Type", "application/json")
			// id приходит числом — клиент должен уметь читать и числа, и строки.
			_, _ = w.Write([]byte(`{"id":42,"login":"ivan","display_name":"Иван Иванов","default_email":"ivan@ya.ru"}`))
		},
	)

	ctx := context.Background()
	token, err := y.ExchangeCode(ctx, "code-123")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if token != "tok-123" {
		t.Fatalf("токен = %q, ждали tok-123", token)
	}

	p, err := y.FetchProfile(ctx, token)
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if gotAuthHeader != "OAuth tok-123" {
		t.Fatalf("Authorization = %q, ждали «OAuth tok-123»", gotAuthHeader)
	}
	if p.ExternalID != "42" || p.Email != "ivan@ya.ru" || p.DisplayName != "Иван Иванов" {
		t.Fatalf("профиль разобран неверно: %+v", p)
	}

	// HandleCallback — связка «обмен + профиль» для AuthProvider.
	p2, err := y.HandleCallback(ctx, "code-123")
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}
	if p2 != p {
		t.Fatalf("HandleCallback: %+v != %+v", p2, p)
	}
}

func TestYandexAuthURL(t *testing.T) {
	y := &YandexClient{
		ClientID:    "cid",
		RedirectURI: "http://localhost:3000/callback",
		AuthURLBase: "https://oauth.yandex.ru/authorize",
	}
	raw, err := y.AuthURL("st-1")
	if err != nil {
		t.Fatalf("AuthURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("разбор URL: %v", err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" || q.Get("client_id") != "cid" ||
		q.Get("state") != "st-1" || q.Get("redirect_uri") != "http://localhost:3000/callback" {
		t.Fatalf("неожиданные параметры AuthURL: %s", raw)
	}

	// Без client_id — понятная русская ошибка.
	if _, err := (&YandexClient{}).AuthURL("st"); err == nil {
		t.Fatal("AuthURL без client_id должен вернуть ошибку")
	} else if !strings.Contains(err.Error(), "YANDEX_CLIENT_ID") {
		t.Fatalf("текст ошибки: %v", err)
	}
}

func TestYandexErrorsAreRussian(t *testing.T) {
	// invalid_grant → понятный текст, без внутренностей провайдера.
	y := newTestYandex(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Code has expired"}`))
		},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	)
	_, err := y.ExchangeCode(context.Background(), "old-code")
	var oe *OAuthError
	if !errors.As(err, &oe) {
		t.Fatalf("ожидали *OAuthError, got %v", err)
	}
	if !strings.Contains(err.Error(), "недействителен или просрочен") {
		t.Fatalf("ожидали русский текст про просрочку: %v", err)
	}
	if strings.Contains(err.Error(), "Code has expired") {
		t.Fatalf("внутренности провайдера утекли наружу: %v", err)
	}

	// Пустой код.
	if _, err := y.ExchangeCode(context.Background(), "  "); err == nil ||
		!strings.Contains(err.Error(), "код авторизации пуст") {
		t.Fatalf("пустой код: %v", err)
	}

	// Не заданы креды приложения.
	if _, err := (&YandexClient{}).ExchangeCode(context.Background(), "x"); err == nil ||
		!strings.Contains(err.Error(), "YANDEX_CLIENT_ID") {
		t.Fatalf("нет кредов: %v", err)
	}

	// Профиль: 401 → «access token недействителен».
	y401 := newTestYandex(t,
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
	)
	if _, err := y401.FetchProfile(context.Background(), "bad-token"); err == nil ||
		!strings.Contains(err.Error(), "access token недействителен") {
		t.Fatalf("профиль с 401: %v", err)
	}

	// Профиль: не-JSON тело → «неожиданный ответ», без тела наружу.
	yBad := newTestYandex(t,
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>внутренняя ошибка 500</html>`))
		},
	)
	_, err = yBad.FetchProfile(context.Background(), "tok")
	if err == nil || !strings.Contains(err.Error(), "неожиданный ответ") {
		t.Fatalf("не-JSON профиль: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "внутренняя ошибка") {
		t.Fatalf("тело провайдера утекло наружу: %v", err)
	}

	// Профиль без id — ошибка.
	yNoID := newTestYandex(t,
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"login":"ivan"}`)) },
	)
	if _, err := yNoID.FetchProfile(context.Background(), "tok"); err == nil ||
		!strings.Contains(err.Error(), "идентификатора") {
		t.Fatalf("профиль без id: %v", err)
	}
}

// TestYandexContextCanceled — отменённый контекмент не ходит в сеть и даёт русскую ошибку.
func TestYandexContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	y := &YandexClient{ClientID: "cid", ClientSecret: "sec"} // URL-ы дефолтные, но сети не будет
	if _, err := y.ExchangeCode(ctx, "code"); err == nil ||
		!strings.Contains(err.Error(), "Яндекс недоступен") {
		t.Fatalf("отменённый контекст: %v", err)
	}
}

// TestNewState — state криптостойкий, base64url, без повторов.
func TestNewState(t *testing.T) {
	s1, err := NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	s2, _ := NewState()
	if len(s1) < 32 || s1 == s2 {
		t.Fatalf("state выглядит ненадёжно: %q / %q", s1, s2)
	}
	if strings.ContainsAny(s1, "+/=") {
		t.Fatalf("state не base64url: %q", s1)
	}
}
