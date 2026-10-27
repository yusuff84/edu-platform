package handler

// Страница подтверждения mock-платежа (T09/T13). В отличие от
// payments.MockConfirmHandler, данные берутся из Store: платёж и его статус
// переживают перезапуск API (актуально для Postgres). Реестр провайдера
// остаётся только для CreatePayment/GetStatus/Refund.

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/payments"
)

// MockPaymentPage — GET /pay/mock/{id}: HTML-страница тестовой оплаты.
// Неизвестный id по-прежнему отдаёт 200 с «Платёж не найден» — так же, как
// прежний обработчик, чтобы страница не превращалась в 404 после рестарта.
func (a *App) MockPaymentPage(w http.ResponseWriter, r *http.Request) {
	// P11: вторая линия защиты после условной регистрации маршрута —
	// прямой вызов хендлера вне dev тоже не должен отдавать mock-страницу.
	if !a.Config.IsDev() {
		writeErr(w, http.StatusNotFound, "Страница не найдена")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	data := payments.MockConfirmData{ID: id, IDJSON: jsString(id)}
	// P11 (cross-provider): страница mock не должна показывать чужой платёж
	// (stripe/yookassa). Иначе она раскрывала бы сумму/курс и предлагала
	// «подтвердить» платёж, который mock-вебхук всё равно отклонит (404).
	if p, ok := a.Store.GetPaymentByProviderID(id); ok && strings.EqualFold(p.Provider, "mock") {
		description := ""
		if c, found := a.Store.GetCourseByID(p.CourseID); found {
			description = c.Title
		}
		data.Amount = fmt.Sprintf("%d.%02d", p.AmountCents/100, p.AmountCents%100)
		data.Currency = p.Currency
		data.Description = description
		data.Status = string(p.Status)
		data.Found = true
	}
	payments.RenderMockConfirm(w, data)
}

// jsString — JSON-строка, безопасная внутри <script> (html/template
// экранирует JS-контекст, но исходное значение должно быть валидным JS).
func jsString(s string) template.JS {
	b, _ := json.Marshal(s)
	return template.JS(b)
}
