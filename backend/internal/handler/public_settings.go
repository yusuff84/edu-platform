package handler

import "net/http"

// publicSettingsResponse — публичные настройки платформы (пост-P7).
// Здесь только значения, безопасные для неавторизованных гостей: ими
// управляет админка, а читает шапка публичных страниц (промо-баннер).
type publicSettingsResponse struct {
	PromoBarEnabled bool `json:"promoBarEnabled"`
}

// GetPublicSettings — GET /settings/public: настройки интерфейса без
// авторизации. Появился вместе с тоглом промо-баннера: Layout читает флаг
// до входа/для гостя, а приватные настройки (комиссия платформы и пр.)
// намеренно сюда не попадают.
func (a *App) GetPublicSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, publicSettingsResponse{
		PromoBarEnabled: a.Store.GetPromoBarEnabled(),
	})
}
