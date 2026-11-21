package handler

import (
	"log"
	"net/http"
	"strings"

	"edu-platform/backend/internal/payments"
	"github.com/go-chi/chi/v5"
)

// AdminSyncPayment reconciles a durable local attempt with its provider after
// a missed webhook or a crash between external creation and local attachment.
func (a *App) AdminSyncPayment(w http.ResponseWriter, r *http.Request) {
	if !a.adminOnly(w, r) {
		return
	}
	a.webhookMu.Lock()
	defer a.webhookMu.Unlock()
	unlock, ok := a.lockBilling(w, r)
	if !ok {
		return
	}
	defer unlock()
	p, ok := a.Store.GetPaymentByID(strings.TrimSpace(chi.URLParam(r, "id")))
	if !ok {
		writeErr(w, http.StatusNotFound, "Платёж не найден")
		return
	}
	provider, err := payments.NewProvider(p.Provider)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Платёжный сервис не настроен")
		return
	}
	if p.ProviderPaymentID == "" {
		res, err := provider.CreatePayment(r.Context(), payments.CreatePaymentRequest{PaymentID: p.ID, AmountCents: p.AmountCents, Currency: p.Currency, Description: p.Description, ReturnURL: p.ReturnURL})
		if err != nil {
			log.Printf("sync checkout %s: %v", p.ID, err)
			writeErr(w, http.StatusBadGateway, "Не удалось восстановить платёж у провайдера")
			return
		}
		if err := a.Store.SetPaymentCheckout(p.ID, res.ProviderPaymentID, res.RedirectURL); err != nil {
			writeErr(w, http.StatusInternalServerError, "Не удалось сохранить результат оплаты")
			return
		}
		p, ok = a.Store.GetPaymentByID(p.ID)
		if !ok {
			writeErr(w, http.StatusInternalServerError, "Не удалось прочитать платёж")
			return
		}
	}
	status, err := provider.GetStatus(r.Context(), p.ProviderPaymentID)
	if err != nil {
		log.Printf("sync payment %s: %v", p.ID, err)
		writeErr(w, http.StatusBadGateway, "Не удалось проверить статус платежа")
		return
	}
	if _, err = a.applyPaymentEvent(p, status); err != nil {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить статус платежа")
		return
	}
	updated, ok := a.Store.GetPaymentByID(p.ID)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "Не удалось прочитать платёж")
		return
	}
	writeJSON(w, http.StatusOK, refundResponse{Payment: toJournalItem(a.journalRowFor(updated))})
}
