package handler

import "net/http"

func (a *App) lockBilling(w http.ResponseWriter, r *http.Request) (func(), bool) {
	unlock, err := a.Store.AcquireBillingLock(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "Платёжная операция временно недоступна, повторите запрос")
		return nil, false
	}
	return unlock, true
}
