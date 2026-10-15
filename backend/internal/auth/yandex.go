package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// Эндпоинты Яндекс ID (https://yandex.ru/dev/id/doc/en):
// authorize — редирект «Войти через Яндекс», token — обмен code на access token,
// info — профиль пользователя по access token.
const (
	yandexAuthURL  = "https://oauth.yandex.ru/authorize"
	yandexTokenURL = "https://oauth.yandex.ru/token"
	yandexInfoURL  = "https://login.yandex.ru/info"
)

// AuthProvider — внешний OAuth-провайдер (Яндекс ID).
type AuthProvider interface {
	// AuthURL возвращает URL редиректа на провайдер.
	AuthURL(state string) (string, error)
	// HandleCallback обменивает code на профиль пользователя.
	HandleCallback(ctx context.Context, code string) (Profile, error)
}

// Profile — нормализованный профиль внешнего пользователя.
type Profile struct {
	ExternalID  string
	Email       string
	DisplayName string
}

// YandexClient — клиент Яндекс ID без внешних SDK (T10).
// Базовые URL вынесены в поля, чтобы подменять их в тестах через httptest.
// Регистрацию/логин через OAuth подключает T13: маршруты
// GET /auth/yandex/start (редирект по AuthURL) и
// GET /auth/yandex/callback (HandleCallback -> find-or-create user -> сессия).
type YandexClient struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTPClient   *http.Client // nil -> дефолтный с DefaultOAuthTimeout
	AuthURLBase  string       // по умолчанию yandexAuthURL
	TokenURL     string       // по умолчанию yandexTokenURL
	InfoURL      string       // по умолчанию yandexInfoURL
}

var _ AuthProvider = (*YandexClient)(nil)

func (y *YandexClient) httpClient() *http.Client {
	return httpClient(y.HTTPClient)
}

// AuthURL строит ссылку «Войти через Яндекс»: response_type=code, client_id,
// redirect_uri (если задан) и state для проверки при callback.
func (y *YandexClient) AuthURL(state string) (string, error) {
	if strings.TrimSpace(y.ClientID) == "" {
		return "", oauthErrorf("формирование ссылки входа", 0, "не задан YANDEX_CLIENT_ID")
	}
	base := y.AuthURLBase
	if base == "" {
		base = yandexAuthURL
	}
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {y.ClientID},
		"state":         {state},
	}
	if strings.TrimSpace(y.RedirectURI) != "" {
		q.Set("redirect_uri", y.RedirectURI)
	}
	return base + "?" + q.Encode(), nil
}

// tokenResponse — разбор ответа POST oauth.yandex.ru/token.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"` // используется только для логирования решений, наружу не идёт
}

// yandexErrorText переводит код ошибки Яндекса на понятный русский текст,
// не раскрывая внутренности провайдера.
func yandexErrorText(code string) string {
	switch code {
	case "invalid_grant":
		return "код авторизации недействителен или просрочен, войдите через Яндекс заново"
	case "invalid_client":
		return "неверные YANDEX_CLIENT_ID или YANDEX_CLIENT_SECRET"
	case "invalid_token":
		return "access token недействителен"
	case "unauthorized_client":
		return "приложение не допущено к этому действию, проверьте настройки Яндекс OAuth"
	}
	return "запрос отклонён Яндексом"
}

// ExchangeCode обменивает authorization code на access token
// (POST https://oauth.yandex.ru/token, grant_type=authorization_code).
func (y *YandexClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	const op = "обмен кода на токен"
	code = strings.TrimSpace(code)
	if code == "" {
		return "", oauthErrorf(op, 0, "код авторизации пуст")
	}
	if strings.TrimSpace(y.ClientID) == "" || strings.TrimSpace(y.ClientSecret) == "" {
		return "", oauthErrorf(op, 0, "не заданы YANDEX_CLIENT_ID или YANDEX_CLIENT_SECRET")
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {y.ClientID},
		"client_secret": {y.ClientSecret},
	}
	if strings.TrimSpace(y.RedirectURI) != "" {
		form.Set("redirect_uri", y.RedirectURI)
	}
	endpoint := y.TokenURL
	if endpoint == "" {
		endpoint = yandexTokenURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", oauthErrorf(op, 0, "не удалось сформировать запрос к Яндексу")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := y.httpClient().Do(req)
	if err != nil {
		// Сетевую ошибку наружу не тащим — только русская суть.
		return "", oauthErrorf(op, 0, "Яндекс недоступен, попробуйте позже")
	}
	body, err := readLimited(resp)
	if err != nil {
		return "", oauthErrorf(op, 0, "не удалось прочитать ответ Яндекса")
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", oauthErrorf(op, resp.StatusCode, "Яндекс вернул неожиданный ответ")
	}
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(tr.AccessToken) == "" {
		return "", oauthErrorf(op, resp.StatusCode, "%s", yandexErrorText(tr.Error))
	}
	return tr.AccessToken, nil
}

// yandexInfoResponse — разбор GET login.yandex.ru/info?format=json.
// id приходит как JSON-число или строка — парсим RawMessage и снимаем кавычки.
type yandexInfoResponse struct {
	ID           json.RawMessage `json:"id"`
	Login        string          `json:"login"`
	DisplayName  string          `json:"display_name"`
	RealName     string          `json:"real_name"`
	DefaultEmail string          `json:"default_email"`
}

// FetchProfile получает профиль пользователя
// (GET https://login.yandex.ru/info?format=json, Authorization: OAuth <token>).
func (y *YandexClient) FetchProfile(ctx context.Context, accessToken string) (Profile, error) {
	const op = "получение профиля"
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return Profile{}, oauthErrorf(op, 0, "access token пуст")
	}
	endpoint := y.InfoURL
	if endpoint == "" {
		endpoint = yandexInfoURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?format=json", nil)
	if err != nil {
		return Profile{}, oauthErrorf(op, 0, "не удалось сформировать запрос к Яндексу")
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)

	resp, err := y.httpClient().Do(req)
	if err != nil {
		return Profile{}, oauthErrorf(op, 0, "Яндекс недоступен, попробуйте позже")
	}
	body, err := readLimited(resp)
	if err != nil {
		return Profile{}, oauthErrorf(op, 0, "не удалось прочитать ответ Яндекса")
	}
	if resp.StatusCode != http.StatusOK {
		return Profile{}, oauthErrorf(op, resp.StatusCode, "%s", yandexErrorText("invalid_token"))
	}
	var info yandexInfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return Profile{}, oauthErrorf(op, resp.StatusCode, "Яндекс вернул неожиданный ответ")
	}

	externalID := strings.Trim(strings.TrimSpace(string(info.ID)), `"`)
	if externalID == "" {
		return Profile{}, oauthErrorf(op, resp.StatusCode, "в профиле Яндекса нет идентификатора пользователя")
	}
	email := strings.TrimSpace(info.DefaultEmail)
	if email == "" && strings.Contains(info.Login, "@") {
		email = info.Login
	}
	name := strings.TrimSpace(info.DisplayName)
	if name == "" {
		name = strings.TrimSpace(info.RealName)
	}
	if name == "" {
		name = email
	}
	return Profile{ExternalID: externalID, Email: email, DisplayName: name}, nil
}

// HandleCallback — связка для AuthProvider: обмен code на токен + профиль.
func (y *YandexClient) HandleCallback(ctx context.Context, code string) (Profile, error) {
	token, err := y.ExchangeCode(ctx, code)
	if err != nil {
		return Profile{}, err
	}
	return y.FetchProfile(ctx, token)
}
