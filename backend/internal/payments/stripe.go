package payments

import (
	"context"
	"errors"
	"net/http"
)

// TODO (оплата): реализовать Stripe-адаптер.
//   - читать STRIPE_SECRET_KEY и STRIPE_WEBHOOK_SECRET из окружения;
//   - CreatePayment через PaymentIntent/Checkout Session;
//   - HandleWebhook с проверкой подписи Stripe-Signature;
//   - маппинг событий payment_intent.succeeded/failed в WebhookEvent.
// Пока все методы возвращают ошибку — провайдер выбран, но не настроен.

// StripeProvider — стаб Stripe для MVP.
type StripeProvider struct{}

// NewStripeProvider создаёт стаб Stripe.
func NewStripeProvider() *StripeProvider { return &StripeProvider{} }

// Name возвращает имя провайдера.
func (s *StripeProvider) Name() string { return "stripe" }

// CreatePayment не реализована в MVP.
func (s *StripeProvider) CreatePayment(_ context.Context, _ CreatePaymentRequest) (CreatePaymentResult, error) {
	return CreatePaymentResult{}, errors.New("stripe: провайдер не настроен в MVP (нет STRIPE_SECRET_KEY)")
}

// HandleWebhook не реализован в MVP.
func (s *StripeProvider) HandleWebhook(_ *http.Request) (WebhookEvent, error) {
	return WebhookEvent{}, errors.New("stripe: webhook не настроен в MVP")
}

// Refund не реализован в MVP.
func (s *StripeProvider) Refund(_ context.Context, _ string) error {
	return errors.New("stripe: возврат не настроен в MVP")
}

// GetStatus не реализован в MVP.
func (s *StripeProvider) GetStatus(_ context.Context, _ string) (string, error) {
	return "", errors.New("stripe: запрос статуса не настроен в MVP")
}
