package handler

// Раздел «Пользователи» супер-админки (T24): список с поиском/фильтрами,
// смена роли, блокировка/разблокировка и удаление учётных записей.
//
// Маршруты монтируются ВНУТРЬ r.Route("/admin", ...) (middleware.go) —
// пути относительные, requireAuth + requireRole(admin) навешаны группой;
// adminOnly в каждом обработчике — страховка, как в admin_payments.go.
//
// Все ошибки-конфликты (защита от запирания) отвечают 409, валидация — 400,
// отсутствие пользователя — 404. Формы JSON — по контракту T24/T26 (фронт
// переиспользует тип строки списка в мутациях).

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
)

// adminUsersMu сериализует изменяющие операции раздела (смена роли,
// статуса, удаление).
//
// Проверки «нельзя трогать последнего администратора» читают состояние
// (CountUsersByRole) и меняют его (SetUserRole/SetUserStatus/DeleteUser)
// двумя отдельными вызовами хранилища. Без сериализации два параллельных
// запроса могли бы одновременно пройти проверку и разжаловать/удалить
// последних админов. Мьютекс делает пару «проверка → запись» атомарной
// в пределах процесса. От нескольких инстансов API защищает только БД —
// объём T24 этого не требует (один процесс = один App).
var adminUsersMu sync.Mutex

// adminUserItem — строка пользователя в списке админки (T24).
// coursesCount — курсы, где пользователь преподаватель; enrollmentsCount —
// его записи на курсы (см. repo.UserDependencies).
type adminUserItem struct {
	ID               string          `json:"id"`
	Email            string          `json:"email"`
	Role             repo.Role       `json:"role"`
	DisplayName      string          `json:"displayName"`
	Status           repo.UserStatus `json:"status"`
	CreatedAt        time.Time       `json:"createdAt"`
	CoursesCount     int             `json:"coursesCount"`
	EnrollmentsCount int             `json:"enrollmentsCount"`
}

// statusOf читает статус пользователя: пустая строка (старые объекты
// in-memory хранилища) трактуется как active (T24).
func statusOf(u *repo.User) repo.UserStatus {
	if u.Status == "" {
		return repo.UserActive
	}
	return u.Status
}

// adminUserItemOf собирает строку ответа; счётчики связей — из UserDependencies.
func (a *App) adminUserItemOf(u *repo.User) adminUserItem {
	deps := a.Store.UserDependencies(u.ID)
	return adminUserItem{
		ID: u.ID, Email: u.Email, Role: u.Role, DisplayName: u.DisplayName,
		Status: statusOf(u), CreatedAt: u.CreatedAt,
		CoursesCount: deps.Courses, EnrollmentsCount: deps.Enrollments,
	}
}

// RegisterAdminUsersRoutes регистрирует маршруты раздела «Пользователи» (T24):
//
//	GET    /admin/users?q=&role=&status=&page=&limit=
//	PATCH  /admin/users/{id}/role
//	PATCH  /admin/users/{id}/status
//	DELETE /admin/users/{id}
func (a *App) RegisterAdminUsersRoutes(r chi.Router) {
	r.Get("/users", a.AdminListUsers)
	r.Patch("/users/{id}/role", a.AdminSetUserRole)
	r.Patch("/users/{id}/status", a.AdminSetUserStatus)
	r.Delete("/users/{id}", a.AdminDeleteUser)
}

// adminUsersPage — конверт списка (items всегда массив, даже пустой).
type adminUsersPage struct {
	Items []adminUserItem `json:"items"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// AdminListUsers — GET /admin/users: фильтрованный список пользователей.
// q — подстрока по email/имени; role/status — точное совпадение ("" —
// «любой»); мусорные значения фильтров — 400. Пагинация — общий paginate.
func (a *App) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	q := r.URL.Query()
	role := repo.Role(strings.TrimSpace(q.Get("role")))
	switch role {
	case "", repo.RoleStudent, repo.RoleTeacher, repo.RoleAdmin:
	default:
		writeErr(w, http.StatusBadRequest, "Некорректная роль: допустимы student, teacher, admin")
		return
	}
	status := repo.UserStatus(strings.TrimSpace(q.Get("status")))
	switch status {
	case "", repo.UserActive, repo.UserBlocked:
	default:
		writeErr(w, http.StatusBadRequest, "Некорректный статус: допустимы active, blocked")
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}

	list := a.Store.ListUsers(repo.UserFilter{
		Q: strings.TrimSpace(q.Get("q")), Role: role, Status: status,
	})
	total := len(list)
	start, page, limit := paginate(total, page, limit)
	end := start + limit
	if end > total {
		end = total
	}
	// make, а не nil-срез: пустой список должен сериализоваться как [], не null.
	items := make([]adminUserItem, 0, end-start)
	for _, u := range list[start:end] {
		items = append(items, a.adminUserItemOf(u))
	}
	writeJSON(w, http.StatusOK, adminUsersPage{Items: items, Total: total, Page: page, Limit: limit})
}

type updateUserRoleRequest struct {
	Role string `json:"role"`
}

// AdminSetUserRole — PATCH /admin/users/{id}/role: смена роли.
// 400 — роль вне student|teacher|admin; 404 — нет пользователя;
// 409 — своя роль или снятие роли с последнего администратора;
// идемпотентно: постановка уже действующей роли — 200.
func (a *App) AdminSetUserRole(w http.ResponseWriter, r *http.Request) {
	var req updateUserRoleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	newRole := repo.Role(strings.TrimSpace(req.Role))
	switch newRole {
	case repo.RoleStudent, repo.RoleTeacher, repo.RoleAdmin:
	default:
		writeErr(w, http.StatusBadRequest, "Некорректная роль: допустимы student, teacher, admin")
		return
	}

	adminUsersMu.Lock()
	defer adminUsersMu.Unlock()
	// adminOnly под мьютексом: роль текущего админа перечитывается из
	// хранилища и не может «устареть» между проверкой и записью.
	if !a.adminOnly(w, r) {
		return
	}
	me, _ := a.currentUser(r)
	target, ok := a.Store.GetUserByID(chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	// Идемпотентность: роль уже такая — ничего не меняется.
	if target.Role == newRole {
		writeJSON(w, http.StatusOK, map[string]any{"user": a.adminUserItemOf(target)})
		return
	}
	// Защита последнего администратора — раньше «своей роли»: если админ
	// в системе один, именно она и есть причина отказа (другого админа,
	// который вернул бы доступ, не существует).
	if target.Role == repo.RoleAdmin && a.Store.CountUsersByRole(repo.RoleAdmin) <= 1 {
		writeErr(w, http.StatusConflict, "Нельзя снять роль администратора с последнего администратора")
		return
	}
	if me != nil && target.ID == me.ID {
		writeErr(w, http.StatusConflict, "Нельзя менять собственную роль")
		return
	}
	if !a.Store.SetUserRole(target.ID, newRole) {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	target.Role = newRole
	writeJSON(w, http.StatusOK, map[string]any{"user": a.adminUserItemOf(target)})
}

type updateUserStatusRequest struct {
	Status string `json:"status"`
}

// AdminSetUserStatus — PATCH /admin/users/{id}/status: блокировка/разблокировка.
// 400 — статус вне active|blocked; 404 — нет пользователя; 409 — своя
// блокировка или блокировка последнего администратора; уже такой статус — 200.
// Сессии при блокировке не удаляются: requireAuth и вход (auth.go) сами
// отсекают заблокированного.
func (a *App) AdminSetUserStatus(w http.ResponseWriter, r *http.Request) {
	var req updateUserStatusRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	newStatus := repo.UserStatus(strings.TrimSpace(req.Status))
	switch newStatus {
	case repo.UserActive, repo.UserBlocked:
	default:
		writeErr(w, http.StatusBadRequest, "Некорректный статус: допустимы active, blocked")
		return
	}

	adminUsersMu.Lock()
	defer adminUsersMu.Unlock()
	if !a.adminOnly(w, r) {
		return
	}
	me, _ := a.currentUser(r)
	target, ok := a.Store.GetUserByID(chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	// Идемпотентность: статус уже такой — ничего не меняется.
	if statusOf(target) == newStatus {
		writeJSON(w, http.StatusOK, map[string]any{"user": a.adminUserItemOf(target)})
		return
	}
	if target.Role == repo.RoleAdmin && newStatus == repo.UserBlocked &&
		a.Store.CountUsersByRole(repo.RoleAdmin) <= 1 {
		writeErr(w, http.StatusConflict, "Нельзя заблокировать последнего администратора")
		return
	}
	if me != nil && target.ID == me.ID && newStatus == repo.UserBlocked {
		writeErr(w, http.StatusConflict, "Нельзя заблокировать собственную учётную запись")
		return
	}
	if !a.Store.SetUserStatus(target.ID, newStatus) {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	target.Status = newStatus
	writeJSON(w, http.StatusOK, map[string]any{"user": a.adminUserItemOf(target)})
}

// AdminDeleteUser — DELETE /admin/users/{id}: удаление учётной записи.
// 404 — нет пользователя; 409 — своя учётка, последний администратор,
// курсы пользователя-преподавателя или платежи (история оплат сохраняется).
// Успех — 204 без тела.
func (a *App) AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	adminUsersMu.Lock()
	defer adminUsersMu.Unlock()
	if !a.adminOnly(w, r) {
		return
	}
	me, _ := a.currentUser(r)
	target, ok := a.Store.GetUserByID(chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if target.Role == repo.RoleAdmin && a.Store.CountUsersByRole(repo.RoleAdmin) <= 1 {
		writeErr(w, http.StatusConflict, "Нельзя удалить последнего администратора")
		return
	}
	if me != nil && target.ID == me.ID {
		writeErr(w, http.StatusConflict, "Нельзя удалить собственную учётную запись")
		return
	}
	deps := a.Store.UserDependencies(target.ID)
	if deps.Courses > 0 {
		writeErr(w, http.StatusConflict, "Нельзя удалить пользователя с курсами")
		return
	}
	if deps.Payments > 0 {
		writeErr(w, http.StatusConflict, "Нельзя удалить пользователя с платежами: история оплат должна сохраниться")
		return
	}
	if err := a.Store.DeleteUser(target.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "Пользователь не найден")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить пользователя")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
