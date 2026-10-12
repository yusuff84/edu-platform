package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// MockProvider — рабочая in-memory реализация для MVP и тестов.
// Платежи хранятся в памяти; webhook эмулируется POST с JSON.
type MockProvider struct {
	mu       sync.RWMutex
	statuses map[string]string // providerPaymentID -> succeeded|pending|failed
}

// NewMockProvider создаёт mock-провайдер.
func NewMockProvider() *MockProvider {
	return &MockProvider{statuses: make(map[string]string)}
}

// Name возвращает имя провайдера.
func (m *MockProvider) Name() string { return "mock" }

// CreatePayment создаёт mock-платёж и ссылку на заглушку оплаты.
func (m *MockProvider) CreatePayment(_ context.Context, req CreatePaymentRequest) (CreatePaymentResult, error) {
	if req.AmountCents < 0 {
		return CreatePaymentResult{}, errors.New("mock: отрицательная сумма платежа")
	}
	id := "mock_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	m.mu.Lock()
	m.statuses[id] = "pending"
	m.mu.Unlock()
	return CreatePaymentResult{
		ProviderPaymentID: id,
		RedirectURL:       "/pay/mock/" + id,
	}, nil
}

type mockWebhookBody struct {
	ProviderPaymentID string `json:"providerPaymentId"`
	Status            string `json:"status"`
}

// HandleWebhook разбирает JSON webhook: {"providerPaymentId","status"}.
func (m *MockProvider) HandleWebhook(r *http.Request) (WebhookEvent, error) {
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return WebhookEvent{}, errors.New("mock: не удалось прочитать тело webhook")
	}
	var body mockWebhookBody
	if err := json.Unmarshal(raw, &body); err != nil || body.ProviderPaymentID == "" {
		return WebhookEvent{}, errors.New("mock: некорректный webhook (нужны providerPaymentId и status)")
	}
	return WebhookEvent{
		ProviderPaymentID: body.ProviderPaymentID,
		Status:            NormalizeStatus(body.Status),
		Raw:               raw,
	}, nil
}

// Refund помечает mock-платёж возвращённым.
func (m *MockProvider) Refund(_ context.Context, providerPaymentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.statuses[providerPaymentID]; !ok {
		return errors.New("mock: платёж не найден")
	}
	m.statuses[providerPaymentID] = "refunded"
	return nil
}

// GetStatus возвращает статус mock-платежа.
func (m *MockProvider) GetStatus(_ context.Context, providerPaymentID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, ok := m.statuses[providerPaymentID]
	if !ok {
		return "", errors.New("mock: платёж не найден")
	}
	return st, nil
}
