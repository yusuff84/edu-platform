package auth

// AuthProvider — внешний OAuth-провайдер (Яндекс ID).
// TODO: реализовать обмен code->token и получение профиля
// через https://oauth.yandex.ru/token и https://login.yandex.ru/info.
type AuthProvider interface {
	// AuthURL возвращает URL редиректа на провайдер.
	AuthURL(state string) string
	// HandleCallback обменивает code на профиль пользователя.
	HandleCallback(code string) (Profile, error)
}

// Profile — нормализованный профиль внешнего пользователя.
type Profile struct {
	ExternalID  string
	Email       string
	DisplayName string
}
