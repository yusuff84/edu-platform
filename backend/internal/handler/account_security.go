package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/mailer"
	"edu-platform/backend/internal/repo"
)

const resetAccepted = "Если аккаунт с таким email существует, письмо со ссылкой для сброса пароля будет отправлено. Проверьте почту и папку «Спам»."
const invalidAccountLink = "Ссылка недействительна, уже использована или срок её действия истёк. Запросите новое письмо."

func (a *App) RegisterAccountSecurityRoutes(r chi.Router) {
	r.With(a.rateLimit("account-recovery", a.Config.RateLimitAuthPerMin)).Post("/auth/password/forgot", a.ForgotPassword)
	r.With(a.rateLimit("account-recovery", a.Config.RateLimitAuthPerMin)).Post("/auth/password/reset", a.ResetPassword)
	r.With(a.rateLimit("account-verification", a.Config.RateLimitAuthPerMin)).Post("/auth/email/verify", a.VerifyEmail)
	r.With(a.requireAuth).Get("/me/security", a.AccountSecurity)
	r.With(a.requireAuth, a.rateLimit("account-verification", a.Config.RateLimitAuthPerMin)).Post("/me/email/verification", a.RequestEmailVerification)
	r.With(a.requireAuth).Get("/me/sessions", a.AccountSessions)
	r.With(a.requireAuth).Delete("/me/sessions/{sessionID}", a.RevokeAccountSession)
	r.With(a.requireAuth).Post("/me/sessions/revoke-others", a.RevokeOtherSessions)
}

// Links keep the raw token in the fragment: it never reaches access logs or
// Referer headers. Nothing mutates on GET, including automatic email previews.
func (a *App) accountLink(purpose, token string) (string, error) {
	base := strings.TrimSpace(a.Config.PublicAppURL)
	if base == "" {
		base = "http://localhost:3000"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && (u.Scheme != "http" || !a.Config.IsDev())) {
		return "", errors.New("некорректный PUBLIC_APP_URL для писем аккаунта")
	}
	path := "/verify-email"
	if purpose == repo.PasswordReset {
		path = "/reset-password"
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.Fragment = url.Values{"token": {token}}.Encode()
	return u.String(), nil
}

// Delivery runs outside the HTTP request and never returns account-specific
// delivery failures to a password-recovery caller. Concurrency is bounded.
// Only token hashes are persisted; restart before delivery permits a resend.
func (a *App) sendAccountEmail(_ context.Context, user *repo.User, purpose string) error {
	if a.Mailer == nil {
		return errors.New("отправка писем недоступна")
	}
	token, hash, _, err := auth.GenerateToken()
	if err != nil {
		return err
	}
	link, err := a.accountLink(purpose, token)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	ttl := 24 * time.Hour
	subject := "Подтвердите email — Edu Platform"
	text := "Для подтверждения email откройте ссылку и нажмите «Подтвердить email». Ссылка действует 24 часа."
	if purpose == repo.PasswordReset {
		ttl = 30 * time.Minute
		subject = "Сброс пароля — Edu Platform"
		text = "Для установки нового пароля откройте ссылку. Она действует 30 минут. Если вы не запрашивали сброс, проигнорируйте это письмо."
	}
	if err = a.Store.IssueAccountToken(user.ID, user.PasswordHash, purpose, hash, now, now.Add(ttl)); err != nil {
		return err
	}
	a.accountMailOnce.Do(func() { a.accountMailSlots = make(chan struct{}, 8) })
	select {
	case a.accountMailSlots <- struct{}{}:
	default:
		_ = a.Store.DiscardAccountToken(hash)
		return errors.New("отправка писем занята, повторите позже")
	}
	go func() {
		defer func() { <-a.accountMailSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Mailer.Send(ctx, mailer.Message{To: user.Email, Subject: subject, Text: text + "\n\n" + link + "\n\nEdu Platform"}); err != nil {
			_ = a.Store.DiscardAccountToken(hash)
			log.Printf("account email delivery failed (user=%s, purpose=%s)", user.ID, purpose)
		}
	}()
	return nil
}

func (a *App) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		writeErr(w, http.StatusBadRequest, "Укажите корректный email")
		return
	}
	if a.Mailer == nil {
		writeErr(w, http.StatusServiceUnavailable, "Отправка писем временно недоступна. Обратитесь в поддержку.")
		return
	}
	// Identical public response for unknown, blocked, throttled and valid users.
	if user, ok := a.Store.GetUserByEmail(email); ok && user.Status != repo.UserBlocked {
		if err := a.sendAccountEmail(r.Context(), user, repo.PasswordReset); err != nil && !errors.Is(err, repo.ErrTokenRateLimited) {
			log.Printf("password recovery not queued (user=%s)", user.ID)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]string{"message": resetAccepted})
}

func validAccountToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	for _, c := range token {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (a *App) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if !validAccountToken(request.Token) {
		writeErr(w, http.StatusBadRequest, invalidAccountLink)
		return
	}
	if utf8.RuneCountInString(request.Password) < 8 || len(request.Password) > 72 {
		writeErr(w, http.StatusBadRequest, "Используйте пароль не короче 8 символов. Слишком длинный пароль сократите.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось установить пароль")
		return
	}
	if err = a.Store.ConsumeAccountToken(auth.HashToken(request.Token), repo.PasswordReset, string(hash), time.Now().UTC()); err != nil {
		if errors.Is(err, repo.ErrInvalidAccountToken) {
			writeErr(w, http.StatusBadRequest, invalidAccountLink)
		} else {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить пароль, повторите запрос")
		}
		return
	}
	http.SetCookie(w, a.sessionCookieFor(r, "", -1))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"message": "Пароль изменён. Все сессии завершены. Войдите с новым паролем."})
}

func (a *App) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if !validAccountToken(request.Token) {
		writeErr(w, http.StatusBadRequest, invalidAccountLink)
		return
	}
	if err := a.Store.ConsumeAccountToken(auth.HashToken(request.Token), repo.EmailVerification, "", time.Now().UTC()); err != nil {
		if errors.Is(err, repo.ErrInvalidAccountToken) {
			writeErr(w, http.StatusBadRequest, invalidAccountLink)
		} else {
			writeErr(w, http.StatusInternalServerError, "Не удалось подтвердить email, повторите запрос")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"message": "Email подтверждён."})
}

func (a *App) AccountSecurity(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"email": user.Email, "emailVerified": user.EmailVerifiedAt != nil, "emailVerifiedAt": user.EmailVerifiedAt, "emailDeliveryAvailable": a.Mailer != nil})
}

func (a *App) RequestEmailVerification(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	if user.EmailVerifiedAt != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Email уже подтверждён."})
		return
	}
	if a.Mailer == nil {
		writeErr(w, http.StatusServiceUnavailable, "Отправка писем временно недоступна. Обратитесь в поддержку.")
		return
	}
	if err := a.sendAccountEmail(r.Context(), user, repo.EmailVerification); err != nil {
		if errors.Is(err, repo.ErrTokenRateLimited) {
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "Новое письмо можно запросить через минуту.")
		} else {
			writeErr(w, http.StatusServiceUnavailable, "Не удалось отправить письмо, повторите позже.")
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "Письмо для подтверждения email будет отправлено. Проверьте почту и папку «Спам»."})
}

func (a *App) currentSessionID(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if id, ok := a.parseSessionValue(cookie.Value); ok {
			return id
		}
	}
	return ""
}

type accountSessionView struct {
	ID         string    `json:"id"`
	Current    bool      `json:"current"`
	UserAgent  string    `json:"userAgent"`
	IPAddress  string    `json:"ipAddress"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func (a *App) AccountSessions(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	sessions, err := a.Store.ListAccountSessions(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось загрузить сессии")
		return
	}
	current := a.currentSessionID(r)
	items := make([]accountSessionView, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, accountSessionView{ID: session.PublicID, Current: session.ID == current, UserAgent: session.UserAgent, IPAddress: session.IPAddress, CreatedAt: session.CreatedAt, LastSeenAt: session.LastSeenAt, ExpiresAt: session.ExpiresAt})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) RevokeAccountSession(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	publicID := chi.URLParam(r, "sessionID")
	current, exists := a.Store.GetSession(a.currentSessionID(r))
	isCurrent := exists && current.PublicID == publicID
	found, err := a.Store.RevokeAccountSession(user.ID, publicID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось завершить сессию")
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Сессия не найдена")
		return
	}
	if isCurrent {
		http.SetCookie(w, a.sessionCookieFor(r, "", -1))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	if err := a.Store.RevokeOtherAccountSessions(user.ID, a.currentSessionID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось завершить другие сессии")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
