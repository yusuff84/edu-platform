// Package payments инкапсулирует гибкую оплату через интерфейс PaymentProvider.
//
// Выбор реализации — через env PAYMENTS_PROVIDER (mock|stripe|yookassa).
// MockProvider полностью рабочий для MVP; stripe/yookassa — стабы с TODO.
package payments

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// CreatePaymentRequest — данные для создания платежа у провайдера.
type CreatePaymentRequest struct {
	PaymentID   string // внутренний id платежа
	AmountCents int
	Currency    string
	Description string
	ReturnURL   string
}

// CreatePaymentResult — результат создания платежа.
type CreatePaymentResult struct {
	ProviderPaymentID string
	RedirectURL       string // куда вести ученика для оплаты
}

// WebhookEvent — нормализованное событие от провайдера.
type WebhookEvent struct {
	ProviderPaymentID string
	Status            string // succeeded | failed
	Raw               []byte
}

// PaymentProvider — контракт платёжного провайдера.
type PaymentProvider interface {
	Name() string
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResult, error)
	HandleWebhook(r *http.Request) (WebhookEvent, error)
	Refund(ctx context.Context, providerPaymentID string) error
	GetStatus(ctx context.Context, providerPaymentID string) (string, error)
}

// NewProvider — фабрика провайдеров по имени из env PAYMENTS_PROVIDER.
func NewProvider(name string) (PaymentProvider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return NewMockProvider(), nil
	case "stripe":
		return NewStripeProvider(), nil
	case "yookassa":
		return NewYookassaProvider(), nil
	default:
		return nil, fmt.Errorf("неизвестный платёжный провайдер: %s", name)
	}
}

// NormalizeStatus приводит сырой статус провайдера к succeeded|failed|pending.
func NormalizeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "succeeded", "success", "paid", "succeeded_payment", "payment.succeeded":
		return "succeeded"
	case "failed", "canceled", "cancelled", "payment.failed":
		return "failed"
	default:
		return "pending"
	}
}
