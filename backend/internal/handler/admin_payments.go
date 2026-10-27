package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"edu-platform/backend/internal/payments"
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
	// T47: возврат средств — id внутреннего UUID платежа (не providerPaymentId).
	r.With(a.requireAuth, a.requireRole(repo.RoleAdmin)).
		Post("/admin/payments/{id}/refund", a.AdminRefundPayment)
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
	StudentEmail       string             `json:"studentEmail"`
	StudentName        string             `json:"studentName"`
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
	// T63: применённый промокод и размер скидки (аддитивные поля контракта:
	// promoCode — пустая строка, если кода не было).
	PromoCode     string `json:"promoCode"`
	DiscountCents int    `json:"discountCents"`
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

// toJournalItem переносит строку журнала в форму контракта US6.
// Вынесено из AdminPaymentsJournal, чтобы ответ возврата средств (T47)
// переиспользовал ровно ту же форму элемента (включая обязательные
// studentEmail/studentName).
func toJournalItem(it repo.PaymentJournalRow) paymentJournalItem {
	return paymentJournalItem{
		ID: it.ID, StudentID: it.StudentID,
		// T16 (п.3, расхождение №6): email и имя студента — иначе в админке
		// в колонке «Студент» светился обрезанный UUID. studentName может
		// быть пустым — тогда фронт показывает studentEmail.
		StudentEmail: it.StudentEmail, StudentName: it.StudentDisplayName,
		CourseID: it.CourseID, CourseTitle: it.CourseTitle,
		AmountCents: it.AmountCents, PlatformFeeCents: it.PlatformFeeCents,
		TeacherAmountCents: it.TeacherAmountCents,
		Currency:           it.Currency, Provider: it.Provider,
		ProviderPaymentID: it.ProviderPaymentID, Status: it.Status,
		CreatedAt: it.CreatedAt, ProcessedAt: it.ProcessedAt,
		PromoCode: it.PromoCode, DiscountCents: it.DiscountCents,
	}
}

// journalRowFor собирает строку журнала по одному платежу: название курса,
// email и имя студента — как в списке GET /admin/payments. Нужно ответу
// возврата средств (T47), где studentEmail/studentName обязательны.
func (a *App) journalRowFor(p *repo.Payment) repo.PaymentJournalRow {
	row := repo.PaymentJournalRow{Payment: *p}
	if c, ok := a.Store.GetCourseByID(p.CourseID); ok {
		row.CourseTitle = c.Title
	}
	if u, ok := a.Store.GetUserByID(p.StudentID); ok {
		row.StudentEmail = u.Email
		row.StudentDisplayName = u.DisplayName
	}
	return row
}

// parseJournalPeriod разбирает границы периода from/to журнала платежей (T31).
// Формат — ГГГГ-ММ-ДД (UTC), пустое значение = границы нет. Неверный формат
// → 400 «Дата в формате ГГГГ-ММ-ДД», from позже to → 400 «Начало периода
// позже конца»; в обоих случаях ответ уже записан и ok=false.
func parseJournalPeriod(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	qv := r.URL.Query()
	var err error
	if v := strings.TrimSpace(qv.Get("from")); v != "" {
		from, err = time.Parse("2006-01-02", v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Дата в формате ГГГГ-ММ-ДД")
			return time.Time{}, time.Time{}, false
		}
	}
	if v := strings.TrimSpace(qv.Get("to")); v != "" {
		to, err = time.Parse("2006-01-02", v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Дата в формате ГГГГ-ММ-ДД")
			return time.Time{}, time.Time{}, false
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		writeErr(w, http.StatusBadRequest, "Начало периода позже конца")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

// AdminPaymentsJournal — GET /admin/payments: журнал платежей (US6), только admin.
// Query: page (1-based), limit (20..100), status (pending|succeeded|failed|refunded),
// courseId, q (T31: подстрока по email или displayName студента, без учёта
// регистра), from/to (T31: границы периода в формате ГГГГ-ММ-ДД, включительно).
// totals считаются по тому же отфильтрованному набору, что и total, но без
// пагинации.
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
	// T31: поиск по студенту и период. Пустой q — фильтра нет.
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	from, to, ok := parseJournalPeriod(w, r)
	if !ok {
		return
	}

	items, total, totals, err := a.Store.ListPaymentsJournal(repo.PaymentFilter{
		Page: page, Limit: limit, Status: status, CourseID: courseID,
		Q: searchQuery, From: from, To: to,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось загрузить журнал платежей")
		return
	}
	out := make([]paymentJournalItem, 0, len(items))
	for _, it := range items {
		out = append(out, toJournalItem(it))
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
	// PromoBarEnabled — пост-P7: включён ли промо-баннер «Первый курс
	// со скидкой». Админка переключает, шапка читает через /settings/public.
	PromoBarEnabled bool `json:"promoBarEnabled"`
}

// AdminGetSettings — GET /admin/settings: текущая комиссия платформы (US6)
// и флаг промо-баннера (пост-P7), только admin.
func (a *App) AdminGetSettings(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{
		CommissionPct:   a.Store.GetCommissionPct(),
		PromoBarEnabled: a.Store.GetPromoBarEnabled(),
	})
}

type updateSettingsRequest struct {
	CommissionPct *int `json:"commissionPct"`
	// PromoBarEnabled — пост-P7: частичное обновление, можно прислать одно
	// поле вместо двух (админка шлёт только изменившийся тогл).
	PromoBarEnabled *bool `json:"promoBarEnabled"`
}

// AdminUpdateSettings — PUT /admin/settings: меняет комиссию платформы (US6)
// и/или флаг промо-баннера (пост-P7), только admin. Оба поля необязательны по
// отдельности, но хотя бы одно должно прийти. Валидация комиссии 0..90;
// значения применяются немедленно — Set-методы обновляют кэш в памяти обоих
// сторов (см. store.go, postgres_store.go), перезапуск не нужен. T16 (п.4):
// ошибка записи в БД пробрасывается и превращается в 500 — раньше Postgres её
// глотал, и API отвечал 200, не сохранив ничего.
func (a *App) AdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	var req updateSettingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CommissionPct == nil && req.PromoBarEnabled == nil {
		writeErr(w, http.StatusBadRequest, "Укажите commissionPct или promoBarEnabled")
		return
	}
	if req.CommissionPct != nil {
		if *req.CommissionPct < 0 || *req.CommissionPct > 90 {
			writeErr(w, http.StatusBadRequest, "Комиссия должна быть в диапазоне от 0 до 90")
			return
		}
		if err := a.Store.SetCommissionPct(*req.CommissionPct); err != nil {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить комиссию платформы")
			return
		}
	}
	if req.PromoBarEnabled != nil {
		if err := a.Store.SetPromoBarEnabled(*req.PromoBarEnabled); err != nil {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить настройку промо-баннера")
			return
		}
	}
	// Отвечаем фактическим состоянием обоих значений: при частичном PUT
	// неприсланное поле — текущее, а не «нулевое» (фронт админки опирается).
	writeJSON(w, http.StatusOK, settingsResponse{
		CommissionPct:   a.Store.GetCommissionPct(),
		PromoBarEnabled: a.Store.GetPromoBarEnabled(),
	})
}

// refundResponse — ответ возврата средств: {"payment": <строка журнала>}.
type refundResponse struct {
	Payment paymentJournalItem `json:"payment"`
}

// refundLocks сериализует возвраты по одному платежу ДО вызова провайдера
// (H4): два одновременных запроса не должны дважды дергать Stripe/YooKassa.
// Замок на платеж, а не глобальный: возвраты разных платежей независимы.
var refundLocks = struct {
	sync.Mutex
	items map[string]*refundLock
}{items: make(map[string]*refundLock)}

type refundLock struct {
	mu   sync.Mutex
	refs int
}

// lockPaymentRefund берёт замок платежа; возвращённая функция освобождает
// его и удаляет запись, когда ждущих не осталось (карта не растёт).
func lockPaymentRefund(id string) func() {
	refundLocks.Lock()
	l := refundLocks.items[id]
	if l == nil {
		l = &refundLock{}
		refundLocks.items[id] = l
	}
	l.refs++
	refundLocks.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		refundLocks.Lock()
		l.refs--
		if l.refs == 0 {
			delete(refundLocks.items, id)
		}
		refundLocks.Unlock()
	}
}

// isDurableMockPayment — платёж, созданный MockProvider. Такой платёж
// возвращается только в Store: реестр mock-провайдера живёт в памяти и после
// рестарта API пуст. Для бесплатного платежа providerPaymentId имеет префикс
// free-, поэтому он сюда не попадает и по-прежнему проходит через провайдера
// (контракт T47: возврат бесплатного платежа → 502).
func isDurableMockPayment(p *repo.Payment) bool {
	return p != nil && p.Provider == "mock" &&
		strings.HasPrefix(strings.TrimSpace(p.ProviderPaymentID), payments.MockPaymentIDPrefix)
}

// AdminRefundPayment — POST /admin/payments/{id}/refund: возврат средств (T47),
// только admin. id — внутренний UUID платежа (не providerPaymentId); провайдер
// берётся из поля provider самого платежа. Возврат возможен только из статуса
// succeeded (иначе 409), неизвестный платёж — 404. Ошибка вызова провайдера
// (включая ErrNotConfigured) — 502.
//
// Применение результата — под a.webhookMu, тем же замком, что и вебхук
// (P5, коммит b9a84df): статус перечитывается и меняется в одной критической
// секции, поэтому два параллельных возврата одного платежа дают ровно один
// 200, второй — 409. Сам вызов провайдера — ВНЕ замка: он может быть сетевым
// и долгим (stripe/yookassa), а гонку сериализует повторная проверка ниже.
func (a *App) AdminRefundPayment(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	// H4: сериализуем возврат по платежу до обращения к провайдеру. Второй
	// параллельный запрос дождётся первого и увидит уже refunded → 409,
	// не выполняя повторный возврат у Stripe/YooKassa.
	unlockRefund := lockPaymentRefund(id)
	defer unlockRefund()

	p, found := a.Store.GetPaymentByID(id)
	if !found {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	if !canTransition(p.Status, repo.PaymentRefunded) {
		writeErr(w, http.StatusConflict, "Возврат возможен только для успешного платежа")
		return
	}

	// Настоящий mock-платёж (mock_...) можно вернуть без обращения к реестру
	// провайдера: денег вне платформы нет, а реестр после рестарта API пуст.
	// Для free-... и реальных провайдеров вызов остаётся обязательным.
	if !isDurableMockPayment(p) {
		prov, err := payments.NewProvider(p.Provider, a.Config.IsDev())
		if err != nil {
			writeErr(w, http.StatusBadGateway, "Не удалось выполнить возврат у провайдера")
			return
		}
		// Ключ refund-<внутренний id> стабилен между ретраями админки; у
		// YooKassa/Stripe он не даст создать второй возврат по тому же платежу.
		if err := prov.Refund(r.Context(), p.ProviderPaymentID, "refund-"+p.ID); err != nil {
			writeErr(w, http.StatusBadGateway, "Не удалось выполнить возврат у провайдера")
			return
		}
	}

	a.webhookMu.Lock()
	defer a.webhookMu.Unlock()

	// Свежее чтение внутри замка: пока шёл вызов провайдера, статус мог
	// измениться (параллельный возврат или вебхук) — тогда 409.
	fresh, found := a.Store.GetPaymentByID(p.ID)
	if !found {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	if !canTransition(fresh.Status, repo.PaymentRefunded) {
		writeErr(w, http.StatusConflict, "Возврат возможен только для успешного платежа")
		return
	}
	if !a.Store.SetPaymentStatus(fresh.ID, repo.PaymentRefunded) {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить статус платежа")
		return
	}
	// Возврат средств отменяет доступ: запись на курс становится refunded
	// (та же семантика, что у ветки refunded в PaymentWebhook, enroll.go).
	// ErrNotFound не считается сбоем (записи нет — отзывать нечего);
	// остальные ошибки записи — 500, чтобы не потерять их молча.
	if err := a.Store.SetEnrollmentStatus(fresh.StudentID, fresh.CourseID, repo.EnrollmentRefunded); err != nil && !errors.Is(err, repo.ErrNotFound) {
		writeErr(w, http.StatusInternalServerError, "Не удалось закрыть доступ к курсу")
		return
	}
	// Свежие значения ПОСЛЕ записи: в ответе — актуальный статус refunded
	// и processedAt (момент возврата; в Postgres снимок нужно перечитать).
	updated, ok := a.Store.GetPaymentByID(fresh.ID)
	if !ok || updated == nil {
		updated = fresh
	}
	writeJSON(w, http.StatusOK, refundResponse{
		Payment: toJournalItem(a.journalRowFor(updated)),
	})
}
