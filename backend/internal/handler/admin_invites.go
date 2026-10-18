package handler

// T47 — админский список приглашений и отзыв приглашения:
//   GET  /admin/invites          — список (createdAt DESC, page/limit);
//   POST /admin/invites/{id}/revoke — отзыв из статуса sent (200/404/409).
//
// Форма ответов заморожена в Task/00-api-contract.md («Эндпоинты волны P6»):
// элемент списка несёт tokenPrefix, но НЕ token/tokenHash — после создания
// сырой токен невосстановим (в БД хранится только хеш), в списке виден
// только префикс. `status` — сырой статус хранилища (sent | accepted |
// expired | revoked); «истекло» фронт вычисляет сам по expiresAt.

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// inviteListItem — элемент списка приглашений в форме контракта T47.
type inviteListItem struct {
	ID          string            `json:"id"`
	Email       string            `json:"email"`
	Status      repo.InviteStatus `json:"status"`
	TokenPrefix string            `json:"tokenPrefix"`
	ExpiresAt   time.Time         `json:"expiresAt"`
	CreatedAt   time.Time         `json:"createdAt"`
	CreatedBy   string            `json:"createdBy"`
}

// invitesListResponse — конверт списка: items/total/page/limit.
type invitesListResponse struct {
	Items []inviteListItem `json:"items"`
	Total int              `json:"total"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
}

// toInviteListItem переносит приглашение в форму контракта. Поля token и
// tokenHash намеренно отсутствуют: наружу уходит только префикс токена.
func toInviteListItem(inv *repo.TeacherInvite) inviteListItem {
	return inviteListItem{
		ID:          inv.ID,
		Email:       inv.Email,
		Status:      inv.Status,
		TokenPrefix: inv.TokenPrefix,
		ExpiresAt:   inv.ExpiresAt,
		CreatedAt:   inv.CreatedAt,
		CreatedBy:   inv.CreatedBy,
	}
}

// AdminListInvites — GET /admin/invites: список приглашений (T47), только admin.
// Query: page (1-based) и limit (20..100) — разбираются тем же хелпером, что
// и журнал платежей; нечисловые значения → 400 тем же текстом.
func (a *App) AdminListInvites(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}
	invites, total := a.Store.ListInvites(page, limit)
	// items — всегда массив (пустой список не должен сериализоваться в null).
	items := make([]inviteListItem, 0, len(invites))
	for _, inv := range invites {
		items = append(items, toInviteListItem(inv))
	}
	writeJSON(w, http.StatusOK, invitesListResponse{
		Items: items, Total: total, Page: page, Limit: limit,
	})
}

// AdminRevokeInvite — POST /admin/invites/{id}/revoke: отзыв приглашения (T47),
// только admin. Тело не требуется. 200 {"invite": {... со status revoked}};
// 404 «Приглашение не найдено»; 409 «Отозвать можно только активное
// приглашение» — статус не sent (в том числе повторный отзыв).
func (a *App) AdminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	inv, err := a.Store.RevokeInvite(id)
	switch {
	case err == nil:
	case errors.Is(err, repo.ErrNotFound):
		writeErr(w, http.StatusNotFound, "Приглашение не найдено")
		return
	case errors.Is(err, repo.ErrConflict):
		writeErr(w, http.StatusConflict, "Отозвать можно только активное приглашение")
		return
	default:
		writeErr(w, http.StatusInternalServerError, "Не удалось отозвать приглашение")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invite": toInviteListItem(inv)})
}
