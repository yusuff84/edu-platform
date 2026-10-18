package handler

import (
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"edu-platform/backend/internal/auth"
	"edu-platform/backend/internal/repo"
)

// Хеширование паролей: в MVP используется bcrypt (golang.org/x/crypto) вместо
// argon2id ради скорости и простоты; интерфейс password-verify изолирован здесь
// и может быть заменён на argon2id без изменения остальных слоёв.

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	InviteToken string `json:"inviteToken"`
}

// userResponse — единый формат пользователя для контракта API (Task/00-api-contract.md).
// DisplayName — всегда строка (пустая, если не задана), чтобы фронт получал "", а не null.
type userResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Role        repo.Role `json:"role"`
	DisplayName string    `json:"displayName"`
}

func userResponseOf(u *repo.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, DisplayName: u.DisplayName}
}

// blockedUserMessage — единый русский текст отказа заблокированному
// пользователю (T24). Все точки входа — парольный вход, /auth/me, callback
// Яндекса (T32) — отвечают им же: фронтенд показывает пользователю одно и то
// же сообщение, а по 403 нельзя различить способ входа.
const blockedUserMessage = "Учётная запись заблокирована"

// writeBlockedUser отвечает 403 с текстом blockedUserMessage (T24, T32).
func writeBlockedUser(w http.ResponseWriter) {
	writeErr(w, http.StatusForbidden, blockedUserMessage)
}

// Register — POST /auth/register. inviteToken повышает роль до teacher.
func (a *App) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || len(email) < 5 {
		writeErr(w, http.StatusBadRequest, "Некорректный email")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "Пароль должен содержать минимум 8 символов")
		return
	}

	role := repo.RoleStudent
	var invite *repo.TeacherInvite
	if strings.TrimSpace(req.InviteToken) != "" {
		inv, ok := a.Store.GetInviteByTokenHash(auth.HashToken(strings.TrimSpace(req.InviteToken)))
		if !ok || inv.Status != repo.InviteSent {
			writeErr(w, http.StatusBadRequest, "Приглашение недействительно или уже использовано")
			return
		}
		if time.Now().UTC().After(inv.ExpiresAt) {
			writeErr(w, http.StatusBadRequest, "Срок действия приглашения истёк")
			return
		}
		if !strings.EqualFold(inv.Email, email) {
			writeErr(w, http.StatusBadRequest, "Приглашение выдано на другой email")
			return
		}
		role = repo.RoleTeacher
		invite = inv
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось создать пользователя, попробуйте позже")
		return
	}
	u, err := a.Store.CreateUser(email, string(hash), role)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Пользователь с таким email уже существует")
		return
	}
	if invite != nil {
		a.Store.AcceptInvite(invite)
	}
	a.startSession(w, r, u.ID)
	writeJSON(w, http.StatusCreated, userResponseOf(u))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login — POST /auth/login, ставит подписанную httpOnly cookie.
func (a *App) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, ok := a.Store.GetUserByEmail(req.Email)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Неверный email или пароль")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		writeErr(w, http.StatusUnauthorized, "Неверный email или пароль")
		return
	}
	// T24: заблокированный не получает сессию — иначе она сразу оказалась бы
	// нерабочей (requireAuth отвечает 403). Пароль проверен ДО статуса, чтобы
	// по 403 нельзя было определить существование чужого аккаунта.
	if u.Status == repo.UserBlocked {
		writeBlockedUser(w)
		return
	}
	a.startSession(w, r, u.ID)
	writeJSON(w, http.StatusOK, userResponseOf(u))
}

// Logout — POST /auth/logout, удаляет сессию и чистит cookie.
// Отвечает 200 {"status":"ok"}: контракт обещает 204, но существующий
// контрактный тест (tests/contract/auth_test.go) требует 200, а тесты
// принадлежат T12; фронтенд обрабатывает оба варианта. См. отчёт T10.
func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me — GET /auth/me, текущий пользователь.
// T24: заблокированный получает 403 и здесь — маршрут не накрыт requireAuth
// (optionalUser статус не проверяет), а сессия перестаёт пускать везде.
func (a *App) Me(w http.ResponseWriter, r *http.Request) {
	u := a.optionalUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return
	}
	if u.Status == repo.UserBlocked {
		writeBlockedUser(w)
		return
	}
	writeJSON(w, http.StatusOK, userResponseOf(u))
}
