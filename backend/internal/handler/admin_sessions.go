package handler

// Раздел «Сессии пользователя» супер-админки (T93): просмотр активных
// cookie-сессий и принудительный выход.
//
// Маршруты монтируются ВНУТРЬ r.Route("/admin", ...) (middleware.go) — пути
// относительные, requireAuth + requireRole(admin) навешаны группой;
// adminOnly в каждом обработчике — страховка, как в admin_users.go.
//
// Наружу отдаются только id сессии, createdAt и expiresAt. Токен/подпись
// cookie в Session не хранятся и в JSON не попадают; id сессий не логируются.

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// adminSessionItem — элемент ответа GET /admin/users/{id}/sessions (T93).
type adminSessionItem struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// adminSessionsResponse — конверт списка сессий (items всегда массив).
type adminSessionsResponse struct {
	Items []adminSessionItem `json:"items"`
}

// RegisterAdminSessionsRoutes регистрирует маршруты сессий пользователя (T93):
//
//	GET    /admin/users/{id}/sessions
//	DELETE /admin/users/{id}/sessions
func (a *App) RegisterAdminSessionsRoutes(r chi.Router) {
	r.Get("/users/{id}/sessions", a.AdminListUserSessions)
	r.Delete("/users/{id}/sessions", a.AdminDeleteUserSessions)
}

// AdminListUserSessions — GET /admin/users/{id}/sessions: активные сессии
// пользователя, новые первыми. 404 — пользователя нет; 401/403 — гварды
// adminOnly. items — всегда массив (пустой список — [], не null).
func (a *App) AdminListUserSessions(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	userID := chi.URLParam(r, "id")
	if _, ok := a.Store.GetUserByID(userID); !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	sessions := a.Store.ListSessionsByUser(userID)
	items := make([]adminSessionItem, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, adminSessionItem{
			ID:        sess.ID,
			CreatedAt: sess.CreatedAt,
			ExpiresAt: sess.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, adminSessionsResponse{Items: items})
}

// AdminDeleteUserSessions — DELETE /admin/users/{id}/sessions: принудительный
// выход — удаляет все сессии пользователя, включая текущую, если админ
// завершает свои. 404 — пользователя нет; успех — 204 без тела.
//
// adminUsersMu здесь не нужен: отзыв сессий не меняет инварианты ролей и
// статусов, которые защищает этот мьютекс (см. admin_users.go).
func (a *App) AdminDeleteUserSessions(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	userID := chi.URLParam(r, "id")
	if _, ok := a.Store.GetUserByID(userID); !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	// Количество — только для лога: ни id пользователя, ни id сессий в
	// открытом виде сверх ответа не пишем.
	if n := a.Store.DeleteSessionsByUser(userID); n > 0 {
		log.Printf("admin: принудительный выход, отозвано сессий: %d", n)
	}
	w.WriteHeader(http.StatusNoContent)
}
