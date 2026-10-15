package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/repo"
)

// RegisterAdminPaymentsRoutes регистрирует админ-эндпоинты US6 (журнал
// платежей и настройка комиссии). Middleware requireAuth + requireRole(admin)
// вешаются здесь же через .With, поэтому маршруты безопасны при монтировании
// на уровне /api/v1 (см. отчёт T07 для T13 — путь монтирования критичен:
// регистрируются абсолютные относительно mount-точки пути /admin/*).
func (a *App) RegisterAdminPaymentsRoutes(r chi.Router) {
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Get("/admin/payments", a.AdminPaymentsJournal)
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Get("/admin/settings", a.AdminGetSettings)
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Put("/admin/settings", a.AdminUpdateSettings)
}

// adminOnly — страховка к requireRole: проверяет, что запрос пришёл от админа,
// даже если маршруты смонтированы вне ожидаемой middleware-цепочки.
func (a *App) adminOnly(w http.ResponseWriter, r *http.Request) bool {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Требуется вход в аккаунт")
		return false
	}
	if u.Role != repo.RoleAdmin {
		writeErr(w, http.StatusForbidden, "Недостаточно прав для этого действия")
		return false
	}
	return true
}

// parseJournalPagination разбирает и валидирует query-параметры page/limit:
// limit по умолчанию 20, максимум 100; page 1-based (по умолчанию 1).
// Нечисловые значения — ошибка валидации.
func parseJournalPagination(r *http.Request) (page, limit int, ok bool) {
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, false
		}
		page = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, false
		}
		limit = n
	}
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit, true
}

// paymentJournalItem — строка журнала в форме контракта US6.
// repo.Payment не имеет JSON-тегов, поэтому поля переносятся явно.
type paymentJournalItem struct {
	ID                 string             `json:"id"`
	StudentID          string             `json:"studentId"`
	CourseID           string             `json:"courseId"`
	CourseTitle        string             `json:"courseTitle"`
	AmountCents        int                `json:"amountCents"`
	PlatformFeeCents   int                `json:"platformFeeCents"`
	TeacherAmountCents int                `json:"teacherAmountCents"`
	Currency           string             `json:"currency"`
	Provider           string             `json:"provider"`
	ProviderPaymentID  string             `json:"providerPaymentId"`
	Status             repo.PaymentStatus `json:"status"`
	CreatedAt          time.Time          `json:"createdAt"`
	ProcessedAt        *time.Time         `json:"processedAt"`
}

type paymentJournalTotals struct {
	AmountCents        int `json:"amountCents"`
	PlatformFeeCents   int `json:"platformFeeCents"`
	TeacherAmountCents int `json:"teacherAmountCents"`
}

type paymentJournalResponse struct {
	Items  []paymentJournalItem `json:"items"`
	Total  int                  `json:"total"`
	Page   int                  `json:"page"`
	Limit  int                  `json:"limit"`
	Totals paymentJournalTotals `json:"totals"`
}

// AdminPaymentsJournal — GET /admin/payments: журнал платежей (US6), только admin.
// Query: page (1-based), limit (20..100), status (pending|succeeded|failed|refunded),
// courseId. totals считаются по всем записям с учётом фильтров, но без пагинации.
func (a *App) AdminPaymentsJournal(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	page, limit, ok := parseJournalPagination(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Параметры page и limit должны быть целыми числами")
		return
	}
	status := repo.PaymentStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", repo.PaymentPending, repo.PaymentSucceeded, repo.PaymentFailed, repo.PaymentRefunded:
	default:
		writeErr(w, http.StatusBadRequest, "Некорректный статус: допустимы pending, succeeded, failed, refunded")
		return
	}
	courseID := strings.TrimSpace(r.URL.Query().Get("courseId"))
	if courseID != "" {
		if _, err := uuid.Parse(courseID); err != nil {
			writeErr(w, http.StatusBadRequest, "Параметр courseId должен быть UUID")
			return
		}
	}

	items, total, totals, err := a.Store.ListPaymentsJournal(repo.PaymentFilter{
		Page: page, Limit: limit, Status: status, CourseID: courseID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось загрузить журнал платежей")
		return
	}
	out := make([]paymentJournalItem, 0, len(items))
	for _, it := range items {
		out = append(out, paymentJournalItem{
			ID: it.ID, StudentID: it.StudentID, CourseID: it.CourseID,
			CourseTitle: it.CourseTitle,
			AmountCents: it.AmountCents, PlatformFeeCents: it.PlatformFeeCents,
			TeacherAmountCents: it.TeacherAmountCents,
			Currency:           it.Currency, Provider: it.Provider,
			ProviderPaymentID: it.ProviderPaymentID, Status: it.Status,
			CreatedAt: it.CreatedAt, ProcessedAt: it.ProcessedAt,
		})
	}
	writeJSON(w, http.StatusOK, paymentJournalResponse{
		Items: out, Total: total, Page: page, Limit: limit,
		Totals: paymentJournalTotals{
			AmountCents: totals.AmountCents, PlatformFeeCents: totals.PlatformFeeCents,
			TeacherAmountCents: totals.TeacherAmountCents,
		},
	})
}

type settingsResponse struct {
	CommissionPct int `json:"commissionPct"`
}

// AdminGetSettings — GET /admin/settings: текущая комиссия платформы (US6),
// только admin.
func (a *App) AdminGetSettings(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{CommissionPct: a.Store.GetCommissionPct()})
}

type updateSettingsRequest struct {
	CommissionPct *int `json:"commissionPct"`
}

// AdminUpdateSettings — PUT /admin/settings: меняет комиссию платформы (US6),
// только admin. Валидация 0..90; значение применяется немедленно —
// SetCommissionPct обновляет кэш в памяти обоих сторов (см. store.go,
// postgres_store.go), перезапуск не нужен.
func (a *App) AdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	var req updateSettingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CommissionPct == nil {
		writeErr(w, http.StatusBadRequest, "Укажите commissionPct")
		return
	}
	if *req.CommissionPct < 0 || *req.CommissionPct > 90 {
		writeErr(w, http.StatusBadRequest, "Комиссия должна быть в диапазоне от 0 до 90")
		return
	}
	a.Store.SetCommissionPct(*req.CommissionPct)
	writeJSON(w, http.StatusOK, settingsResponse{CommissionPct: *req.CommissionPct})
}
