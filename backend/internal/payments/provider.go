// Package payments инкапсулирует гибкую оплату через интерфейс PaymentProvider.
//
// Выбор реализации — через env PAYMENTS_PROVIDER (mock|stripe|yookassa).
// MockProvider полностью рабочий для MVP (страница подтверждения /pay/mock/{id});
// stripe и yookassa работают через net/http без SDK (Docker-First, минимум зависимостей).
package payments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Sentinel-ошибки пакета: позволяют слою handler различать
// «провайдер не настроен», «неверная подпись» и «сбой внешнего API».
var (
	// ErrNotConfigured — провайдер выбран, но не настроен (нет ключей в env).
	ErrNotConfigured = errors.New("платёжный провайдер не настроен")
	// ErrUnauthorized — неверная или отсутствующая подпись вебхука.
	ErrUnauthorized = errors.New("неверная подпись вебхука")
	// ErrUpstream — ошибка обращения к внешнему API провайдера.
	ErrUpstream = errors.New("ошибка внешнего платёжного сервиса")
)

// maxWebhookBody — лимит тела вебхука (1 МБ, как у decodeJSON).
const maxWebhookBody = 1 << 20

// CreatePaymentRequest — данные для создания платежа у провайдера.
type CreatePaymentRequest struct {
	PaymentID   string // внутренний id платежа (используется как ключ идемпотентности)
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
	Status            string // succeeded | failed | refunded | pending
	Raw               []byte
}

// PaymentProvider — контракт платёжного провайдера.
//
// VerifyWebhook добавлен в интерфейс (T09): подпись проверяется хендлером
// ДО разбора события. Все три реализации находятся в этом пакете,
// поэтому изменение интерфейса не ломает внешний код: фабрика NewProvider —
// единственная точка создания провайдеров.
type PaymentProvider interface {
	Name() string
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResult, error)
	// VerifyWebhook проверяет подпись/целостность тела запроса провайдера.
	// Параметр secret позволяет подменить секрет (для тестов); при nil
	// провайдер использует свой секрет из env, прочитанный в конструкторе.
	VerifyWebhook(r *http.Request, secret []byte) error
	HandleWebhook(r *http.Request) (WebhookEvent, error)
	Refund(ctx context.Context, providerPaymentID string) error
	GetStatus(ctx context.Context, providerPaymentID string) (string, error)
}

// NewProvider — фабрика провайдеров по имени из env PAYMENTS_PROVIDER.
// Конструкторы stripe/yookassa возвращают ErrNotConfigured, если ключи не заданы:
// это ошибка конфигурации, а не молчаливая заглушка.
func NewProvider(name string) (PaymentProvider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return NewMockProvider(), nil
	case "stripe":
		return NewStripeProvider()
	case "yookassa":
		return NewYookassaProvider()
	default:
		return nil, fmt.Errorf("неизвестный платёжный провайдер: %s", name)
	}
}

// NormalizeStatus приводит сырой статус провайдера к succeeded|failed|refunded|pending.
func NormalizeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "succeeded", "success", "paid", "succeeded_payment", "payment.succeeded":
		return "succeeded"
	case "failed", "canceled", "cancelled", "payment.failed":
		return "failed"
	case "refunded", "refund", "payment.refunded", "refund.succeeded":
		return "refunded"
	default:
		return "pending"
	}
}

// readRawBody читает сырое тело запроса (до какого-либо парсинга) и
// восстанавливает r.Body, чтобы его можно было прочитать повторно.
// Подпись Stripe проверяется именно по сырому телу — классическая ошибка
// в том, что подпись считают по пере-сериализованному JSON.
func readRawBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать тело вебхука: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, nil
}

// DefaultReturnURL — куда вести ученика после оплаты (env PAYMENTS_RETURN_URL).
// Stripe и YooKassa требуют абсолютный URL возврата.
func DefaultReturnURL() string {
	if v := strings.TrimSpace(os.Getenv("PAYMENTS_RETURN_URL")); v != "" {
		return v
	}
	return "http://localhost:3000/"
}

// returnURL — URL возврата: явный из запроса, иначе из env.
func returnURL(explicit string) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return v
	}
	return DefaultReturnURL()
}

// ResumeURL восстанавливает ссылку на оплату при повторном запросе покупки
// (идемпотентность POST /enrollments при pending-платеже).
func ResumeURL(provider, providerPaymentID string) string {
	if provider == "mock" {
		return MockConfirmPath(providerPaymentID)
	}
	// Для stripe/yookassa ссылка не хранится и не восстанавливается
	// без запроса к их API — фронтенд создаёт новый платёж.
	return ""
}
