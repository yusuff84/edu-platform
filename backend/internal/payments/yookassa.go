package payments

import (
	"context"
	"errors"
	"net/http"
)

// TODO (оплата): реализовать YooKassa-адаптер.
//   - читать YOOKASSA_SHOP_ID и YOOKASSA_SECRET_KEY из окружения;
//   - CreatePayment через POST /v3/payments с ключом идемпотентности;
//   - HandleWebhook с проверкой источника и объекта payment.succeeded/waiting_for_capture;
//   - маппинг waiting_for_capture -> succeeded после capture.
// Пока все методы возвращают ошибку — провайдер выбран, но не настроен.

// YookassaProvider — стаб YooKassa для MVP.
type YookassaProvider struct{}

// NewYookassaProvider создаёт стаб YooKassa.
func NewYookassaProvider() *YookassaProvider { return &YookassaProvider{} }

// Name возвращает имя провайдера.
func (y *YookassaProvider) Name() string { return "yookassa" }

// CreatePayment не реализована в MVP.
func (y *YookassaProvider) CreatePayment(_ context.Context, _ CreatePaymentRequest) (CreatePaymentResult, error) {
	return CreatePaymentResult{}, errors.New("yookassa: провайдер не настроен в MVP (нет YOOKASSA_SHOP_ID)")
}

// HandleWebhook не реализован в MVP.
func (y *YookassaProvider) HandleWebhook(_ *http.Request) (WebhookEvent, error) {
	return WebhookEvent{}, errors.New("yookassa: webhook не настроен в MVP")
}

// Refund не реализован в MVP.
func (y *YookassaProvider) Refund(_ context.Context, _ string) error {
	return errors.New("yookassa: возврат не настроен в MVP")
}

// GetStatus не реализован в MVP.
func (y *YookassaProvider) GetStatus(_ context.Context, _ string) (string, error) {
	return "", errors.New("yookassa: запрос статуса не настроен в MVP")
}
