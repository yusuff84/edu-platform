package contract_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/mailer"
	"edu-platform/backend/internal/repo"
)

type accountMailbox struct {
	messages chan mailer.Message
	fail     bool
}

type failingAccountSessionStore struct{ repo.StoreAPI }

func (s failingAccountSessionStore) CreateAccountSession(string, string, time.Duration, string, string) (*repo.Session, error) {
	return nil, errors.New("injected session storage failure")
}

func TestAccountSessionStorageFailureDoesNotIssueCookie(t *testing.T) {
	app, router := newTestApp()
	registerUser(t, router, "session-storage@example.com", "password123", "")
	app.Store = failingAccountSessionStore{app.Store}
	result := doRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "session-storage@example.com", "password": "password123"}, nil)
	if result.Code != 503 || len(result.Result().Cookies()) != 0 {
		t.Fatalf("session created without persistence: %d %s", result.Code, result.Body.String())
	}
}

func (m *accountMailbox) Send(_ context.Context, message mailer.Message) error {
	if m.fail {
		return errors.New("injected SMTP error")
	}
	m.messages <- message
	return nil
}
func mailbox(t *testing.T) *accountMailbox {
	t.Helper()
	return &accountMailbox{messages: make(chan mailer.Message, 32)}
}
func accountMessage(t *testing.T, m *accountMailbox) mailer.Message {
	t.Helper()
	select {
	case message := <-m.messages:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("email was not delivered")
		return mailer.Message{}
	}
}
func messageToken(t *testing.T, message mailer.Message) (string, string) {
	t.Helper()
	for _, line := range strings.Split(message.Text, "\n") {
		if strings.HasPrefix(line, "http") {
			link, err := url.Parse(line)
			if err != nil {
				t.Fatal(err)
			}
			fragment, err := url.ParseQuery(link.Fragment)
			if err != nil {
				t.Fatal(err)
			}
			token := fragment.Get("token")
			if len(token) != 43 || link.RawQuery != "" {
				t.Fatalf("unsafe token link: %s", line)
			}
			return token, link.Path
		}
	}
	t.Fatal("email has no link")
	return "", ""
}

func TestAccountPasswordResetOneUseAndRevokesAllSessions(t *testing.T) {
	app, router := newTestApp()
	cookies, userID := registerUser(t, router, "reset-account@example.com", "password123", "")
	second := loginUser(t, router, "reset-account@example.com", "password123")
	fake := mailbox(t)
	app.Mailer = fake
	app.Config.PublicAppURL = "https://edu.example"
	unknown := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/forgot", map[string]any{"email": "unknown@example.com"}, nil)
	known := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/forgot", map[string]any{"email": "reset-account@example.com"}, nil)
	if known.Code != 202 || unknown.Code != 202 || known.Body.String() != unknown.Body.String() {
		t.Fatal("password recovery reveals account existence")
	}
	message := accountMessage(t, fake)
	token, path := messageToken(t, message)
	if path != "/reset-password" || message.To != "reset-account@example.com" {
		t.Fatalf("wrong reset email: %s %s", path, message.To)
	}
	if strings.Contains(known.Body.String(), token) {
		t.Fatal("token exposed through recovery API")
	}
	// Reading the page cannot consume the link; a wrong-purpose POST cannot either.
	wrong := doRequest(t, router, http.MethodPost, "/api/v1/auth/email/verify", map[string]any{"token": token}, nil)
	if wrong.Code != 400 {
		t.Fatalf("wrong token purpose: %d", wrong.Code)
	}
	for _, invalidPassword := range []string{"short", strings.Repeat("я", 37)} {
		invalid := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": token, "password": invalidPassword}, nil)
		if invalid.Code != 400 {
			t.Fatalf("invalid password accepted: %d", invalid.Code)
		}
	}
	replay := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/forgot", map[string]any{"email": "reset-account@example.com"}, nil)
	if replay.Code != 202 || replay.Body.String() != known.Body.String() {
		t.Fatal("throttle reveals account existence")
	}
	select {
	case <-fake.messages:
		t.Fatal("per-user resend limit bypassed")
	default:
	}
	result := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": token, "password": "new-password123"}, cookies)
	if result.Code != 200 {
		t.Fatalf("reset: %d %s", result.Code, result.Body.String())
	}
	for _, session := range [][]*http.Cookie{cookies, second} {
		if rec := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, session); rec.Code != 401 {
			t.Fatalf("old session survives reset: %d", rec.Code)
		}
	}
	old := doRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "reset-account@example.com", "password": "password123"}, nil)
	if old.Code != 401 {
		t.Fatalf("old password accepted: %d", old.Code)
	}
	loginUser(t, router, "reset-account@example.com", "new-password123")
	used := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": token, "password": "third-password123"}, nil)
	if used.Code != 400 {
		t.Fatalf("reused reset link: %d", used.Code)
	}
	if sessions, err := app.Store.ListAccountSessions(userID); err != nil || len(sessions) != 1 {
		t.Fatalf("active sessions: %d %v", len(sessions), err)
	}
}

func TestAccountVerificationOnRegistrationAndSingleUse(t *testing.T) {
	app, router := newTestApp()
	fake := mailbox(t)
	app.Mailer = fake
	app.Config.PublicAppURL = "https://edu.example"
	cookies, id := registerUser(t, router, "verify-account@example.com", "password123", "")
	message := accountMessage(t, fake)
	token, path := messageToken(t, message)
	if path != "/verify-email" {
		t.Fatalf("verification path: %s", path)
	}
	if user, _ := app.Store.GetUserByID(id); user.EmailVerifiedAt != nil {
		t.Fatal("registration confirmed email without user action")
	}
	resend := doRequest(t, router, http.MethodPost, "/api/v1/me/email/verification", nil, cookies)
	if resend.Code != 429 {
		t.Fatalf("verification cooldown: %d %s", resend.Code, resend.Body.String())
	}
	wrong := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": token, "password": "new-password123"}, nil)
	if wrong.Code != 400 {
		t.Fatalf("verification token changed password: %d", wrong.Code)
	}
	verified := doRequest(t, router, http.MethodPost, "/api/v1/auth/email/verify", map[string]any{"token": token}, nil)
	if verified.Code != 200 {
		t.Fatalf("verify: %d %s", verified.Code, verified.Body.String())
	}
	if user, _ := app.Store.GetUserByID(id); user.EmailVerifiedAt == nil {
		t.Fatal("email remains unverified")
	}
	me := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, cookies)
	if !strings.Contains(me.Body.String(), `"emailVerified":true`) {
		t.Fatalf("me missing status: %s", me.Body.String())
	}
	used := doRequest(t, router, http.MethodPost, "/api/v1/auth/email/verify", map[string]any{"token": token}, nil)
	if used.Code != 400 {
		t.Fatalf("verification replay: %d", used.Code)
	}
}

func TestAccountSessionsOwnershipAndCurrentLogout(t *testing.T) {
	app, router := newTestApp()
	first, id := registerUser(t, router, "sessions-account@example.com", "password123", "")
	second := loginUser(t, router, "sessions-account@example.com", "password123")
	other, _ := registerUser(t, router, "other-session-account@example.com", "password123", "")
	rec := doRequest(t, router, http.MethodGet, "/api/v1/me/sessions", nil, first)
	var data struct {
		Items []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		}
	}
	decodeBody(t, rec, &data)
	if rec.Code != 200 || len(data.Items) != 2 {
		t.Fatalf("sessions: %d %s", rec.Code, rec.Body.String())
	}
	for _, session := range first {
		if session.Name == "session_id" && strings.Contains(rec.Body.String(), session.Value) {
			t.Fatal("raw session credential exposed")
		}
	}
	current, remote := "", ""
	for _, session := range data.Items {
		if session.Current {
			current = session.ID
		} else {
			remote = session.ID
		}
	}
	if current == "" || remote == "" {
		t.Fatal("current session not identified")
	}
	forged := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{{Name: "session_id", Value: current}})
	if forged.Code != 401 {
		t.Fatal("management ID can be used as an authentication credential")
	}
	foreign := doRequest(t, router, http.MethodDelete, "/api/v1/me/sessions/"+remote, nil, other)
	if foreign.Code != 404 {
		t.Fatalf("foreign revoke: %d", foreign.Code)
	}
	deleted := doRequest(t, router, http.MethodDelete, "/api/v1/me/sessions/"+remote, nil, first)
	if deleted.Code != 204 {
		t.Fatalf("revoke: %d %s", deleted.Code, deleted.Body.String())
	}
	if result := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, second); result.Code != 401 {
		t.Fatal("revoked device remains authenticated")
	}
	third := loginUser(t, router, "sessions-account@example.com", "password123")
	if result := doRequest(t, router, http.MethodPost, "/api/v1/me/sessions/revoke-others", nil, first); result.Code != 204 {
		t.Fatal("revoke others failed")
	}
	if result := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, third); result.Code != 401 {
		t.Fatal("other session survived")
	}
	if sessions, _ := app.Store.ListAccountSessions(id); len(sessions) != 1 {
		t.Fatal("current session not preserved")
	}
	if result := doRequest(t, router, http.MethodDelete, "/api/v1/me/sessions/"+current, nil, first); result.Code != 204 {
		t.Fatal("current revoke failed")
	}
	if result := doRequest(t, router, http.MethodGet, "/api/v1/auth/me", nil, first); result.Code != 401 {
		t.Fatal("current cookie still works")
	}
}

func TestAccountTokenExpiryBlockedUsersAndConcurrentConsume(t *testing.T) {
	app, router := newTestApp()
	_, id := registerUser(t, router, "expired-account@example.com", "password123", "")
	user, _ := app.Store.GetUserByID(id)
	token, hash, _, _ := auth.GenerateToken()
	old := time.Now().Add(-2 * time.Hour)
	if err := app.Store.IssueAccountToken(id, user.PasswordHash, repo.PasswordReset, hash, old, old.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if result := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": token, "password": "new-password123"}, nil); result.Code != 400 {
		t.Fatal("expired token accepted")
	}
	valid, validHash, _, _ := auth.GenerateToken()
	if err := app.Store.IssueAccountToken(id, user.PasswordHash, repo.PasswordReset, validHash, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	app.Store.SetUserStatus(id, repo.UserBlocked)
	if result := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": valid, "password": "new-password123"}, nil); result.Code != 400 {
		t.Fatal("blocked account reset accepted")
	}
	app.Store.SetUserStatus(id, repo.UserActive)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = doRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", map[string]any{"token": valid, "password": "new-password123"}, nil).Code
		}(i)
	}
	wg.Wait()
	if !((codes[0] == 200 && codes[1] == 400) || (codes[0] == 400 && codes[1] == 200)) {
		t.Fatalf("token consumed multiple times: %v", codes)
	}
}

func TestAccountMailUnavailableDoesNotReturnTokens(t *testing.T) {
	app, router := newTestApp()
	cookies, _ := registerUser(t, router, "mail-disabled@example.com", "password123", "")
	for _, path := range []string{"/api/v1/auth/password/forgot", "/api/v1/me/email/verification"} {
		var body any
		if strings.Contains(path, "forgot") {
			body = map[string]any{"email": "mail-disabled@example.com"}
		}
		result := doRequest(t, router, http.MethodPost, path, body, cookies)
		if result.Code != 503 || strings.Contains(result.Body.String(), "token") {
			t.Fatalf("mail unavailable: %d %s", result.Code, result.Body.String())
		}
	}
	app.Mailer = &accountMailbox{messages: make(chan mailer.Message, 8), fail: true}
	result := doRequest(t, router, http.MethodPost, "/api/v1/auth/password/forgot", map[string]any{"email": "mail-disabled@example.com"}, nil)
	if result.Code != 202 {
		t.Fatal("SMTP failure leaked account state")
	}
}
