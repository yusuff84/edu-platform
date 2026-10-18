package handler

// Вход через Яндекс ID (T13: хендлеры поверх auth-клиента из T10).
//
// GET /api/v1/auth/yandex/start   — генерирует state, кладёт в отдельный
//                                    короткоживущий cookie (не session_id)
//                                    и редиректит на AuthURL провайдера;
// GET /api/v1/auth/yandex/callback — сверяет state, очищает cookie, меняет
//                                    code на профиль, find-or-create
//                                    пользователя и открывает сессию.
//
// Клиент YandexClient создаётся один раз в NewRouter (App.YandexAuth);
// поле нужно и для подмены провайдера в тестах. Маршруты регистрируются
// в middleware.go без отдельного лимитера auth — это GET-редиректы,
// действует общий глобальный лимит /api/v1.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/repo"
)

// oauthStateCookie — cookie с state OAuth-редиректа.
const oauthStateCookie = "oauth_state"

// oauthStateTTL — время жизни state-cookie (редирект туда-обратно занимает секунды).
const oauthStateTTL = 10 * time.Minute

// yandexProvider возвращает OAuth-провайдер. Клиент создаётся один раз
// в NewRouter (поле App.YandexAuth) — здесь только fallback на случай,
// если роутер не собирался (прямые вызовы хендлера в тестах).
func (a *App) yandexProvider() auth.AuthProvider {
	if a.YandexAuth != nil {
		return a.YandexAuth
	}
	return &auth.YandexClient{
		ClientID:     a.Config.YandexClientID,
		ClientSecret: a.Config.YandexClientSecret,
		RedirectURI:  a.Config.YandexRedirectURI,
	}
}

// oauthStateCookieFor собирает cookie состояния: httpOnly, SameSite=Lax
// всегда — Яндекс возвращает браузер кросс-site редиректом GET, а при
// SameSite=Strict такой cookie браузер не шлёт. Пустое value — удаление
// cookie (MaxAge=-1): state одноразовый.
func (a *App) oauthStateCookieFor(r *http.Request, value string) *http.Cookie {
	maxAge := int(oauthStateTTL.Seconds())
	if value == "" {
		maxAge = -1
	}
	return &http.Cookie{
		Name:     oauthStateCookie,
		Value:    value,
		Path:     "/api/v1/auth/yandex",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
}

// YandexAuthStart — GET /api/v1/auth/yandex/start: старт входа через Яндекс.
// Без YANDEX_CLIENT_ID отвечает 503 — вход не настроен, это ошибка
// конфигурации, а не 404 (T10-клиент готов, ключи не заданы).
func (a *App) YandexAuthStart(w http.ResponseWriter, r *http.Request) {
	state, err := auth.NewState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось начать вход через Яндекс, попробуйте позже")
		return
	}
	p := a.yandexProvider()
	url, err := p.AuthURL(state)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "Вход через Яндекс не настроен на сервере")
		return
	}
	http.SetCookie(w, a.oauthStateCookieFor(r, state))
	http.Redirect(w, r, url, http.StatusFound)
}

// YandexAuthCallback — GET /api/v1/auth/yandex/callback: завершение входа.
// Сверяется state из cookie (защита от подмены code), затем code меняется
// на профиль; пользователь find-or-create по email, ставится сессия.
func (a *App) YandexAuthCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value == "" ||
		subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(r.URL.Query().Get("state"))) != 1 {
		writeErr(w, http.StatusBadRequest, "Некорректный или просроченный параметр state, начните вход заново")
		return
	}
	// State одноразовый: подчищаем cookie, чтобы повторный callback не проходил.
	http.SetCookie(w, a.oauthStateCookieFor(r, ""))

	code := r.URL.Query().Get("code")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "Яндекс не вернул код авторизации")
		return
	}

	p := a.yandexProvider()
	profile, err := p.HandleCallback(r.Context(), code)
	if err != nil {
		// Ошибка внешнего провайдера — 502, текст из OAuthError (по-русски, без сетевых деталей).
		writeErr(w, http.StatusBadGateway, "Не удалось войти через Яндекс: "+oauthMessage(err))
		return
	}
	// StoreAPI ищет пользователей по email; если Яндекс не вернул email
	// (аккаунт без почты), ключуем пользователя по ExternalID — синтетический
	// адрес стабилен и не конфликтует с реальной почтой (домен зарезервирован).
	email := strings.TrimSpace(profile.Email)
	if email == "" {
		if strings.TrimSpace(profile.ExternalID) == "" {
			writeErr(w, http.StatusBadGateway, "Профиль Яндекса не содержит идентификатора пользователя")
			return
		}
		email = "yandex-" + profile.ExternalID + "@oauth.invalid"
	}

	u, ok := a.Store.GetUserByEmail(email)
	if !ok {
		u, err = a.createOAuthUser(email)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "Не удалось создать пользователя, попробуйте позже")
			return
		}
	}
	// T32: блокировка (T24) действует и на входе через Яндекс — сессии не
	// будет, отказ тот же по коду и тексту, что на парольном входе. В ответе
	// 403 не должно быть ни одной Set-Cookie, поэтому снимаем подготовленную
	// выше очистку state-cookie: вход просто не состоялся.
	if u.Status == repo.UserBlocked {
		w.Header().Del("Set-Cookie")
		writeBlockedUser(w)
		return
	}
	// Имя из профиля Яндекса — только при первом входе (не затираем
	// ручную правку пользователя).
	if u.DisplayName == "" && profile.DisplayName != "" {
		a.Store.SetUserDisplayName(u.ID, profile.DisplayName)
	}
	a.startSession(w, r, u.ID)
	// Фронтенд-хост отличается от API (3000 против 8082), поэтому ведём на
	// корень приложения, а не на /api/v1 — браузер окажется на фронтенде.
	http.Redirect(w, r, "/", http.StatusFound)
}

// createOAuthUser создаёт пользователя по OAuth-профилю: пароль неизвестен,
// поэтому генерируем случайный — вход по паролю невозможен, но bcrypt-хеш
// валиден (пользователь как будто «забыл пароль»).
func (a *App) createOAuthUser(email string) (*repo.User, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, errors.New("не удалось сгенерировать пароль")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(base64.RawURLEncoding.EncodeToString(raw)), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u, err := a.Store.CreateUser(email, string(hash), repo.RoleStudent)
	if err != nil {
		// Гонка: параллельный callback того же email уже создал пользователя.
		if existing, found := a.Store.GetUserByEmail(email); found {
			return existing, nil
		}
		return nil, err
	}
	return u, nil
}

// oauthMessage достаёт русский текст из OAuthError, не таща сетевые детали.
func oauthMessage(err error) string {
	var oe *auth.OAuthError
	if errors.As(err, &oe) {
		return oe.Msg
	}
	return "внешний сервис недоступен, попробуйте позже"
}
