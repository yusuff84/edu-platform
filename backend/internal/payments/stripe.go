package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Stripe-адаптер через net/http без SDK (ограничение Docker-First-конституции).
//   - STRIPE_SECRET_KEY — секретный ключ API (Authorization: Bearer);
//   - STRIPE_WEBHOOK_SECRET — секрет подписи вебхуков (whsec_...).
// Без ключей конструктор возвращает ErrNotConfigured — не молчаливую заглушку.

const (
	// stripeAPIBase — базовый URL Stripe API (в тестах пакета переопределяется).
	stripeAPIBase = "https://api.stripe.com"
	// stripeTimeout — таймаут всех исходящих вызовов к Stripe.
	stripeTimeout = 15 * time.Second
	// stripeSignatureTolerance — допуск по времени подписи (защита от replay).
	stripeSignatureTolerance = 300 * time.Second
)

// StripeProvider — адаптер Stripe.
type StripeProvider struct {
	secretKey     string
	webhookSecret string
	apiBase       string
	client        *http.Client
}

// NewStripeProvider создаёт адаптер Stripe из env.
// Оба ключа обязательны: и для API, и для проверки подписи вебхука.
func NewStripeProvider() (*StripeProvider, error) {
	secretKey := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	webhookSecret := strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if secretKey == "" || webhookSecret == "" {
		return nil, fmt.Errorf("%w: stripe требует STRIPE_SECRET_KEY и STRIPE_WEBHOOK_SECRET", ErrNotConfigured)
	}
	return newStripeProvider(secretKey, webhookSecret), nil
}

// newStripeProvider — конструктор с явными секретами (используется тестами без env).
func newStripeProvider(secretKey, webhookSecret string) *StripeProvider {
	return &StripeProvider{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		apiBase:       stripeAPIBase,
		client:        &http.Client{Timeout: stripeTimeout},
	}
}

// Name возвращает имя провайдера.
func (s *StripeProvider) Name() string { return "stripe" }

// stripePaymentIntent — минимальная модель ответа PaymentIntent.
type stripePaymentIntent struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	NextAction *struct {
		RedirectToURL *struct {
			URL string `json:"url"`
		} `json:"redirect_to_url"`
	} `json:"next_action"`
}

// stripeAPIError разбирает {"error":{"message":"..."}}.
func stripeAPIError(raw []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return string(raw)
}

// CreatePayment создаёт PaymentIntent через POST /v1/payment_intents.
// Сумма приходит в копейках — Stripe также ожидает минорные единицы (unit_amount).
// Idempotency-Key берётся из req.PaymentID (внутренний id попытки покупки).
func (s *StripeProvider) CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResult, error) {
	if s.secretKey == "" {
		return CreatePaymentResult{}, fmt.Errorf("%w: не задан STRIPE_SECRET_KEY", ErrNotConfigured)
	}
	if req.AmountCents <= 0 {
		return CreatePaymentResult{}, errors.New("stripe: сумма платежа должна быть больше нуля")
	}
	form := url.Values{}
	form.Set("amount", strconv.Itoa(req.AmountCents))
	form.Set("currency", strings.ToLower(strings.TrimSpace(req.Currency)))
	form.Set("description", req.Description)
	// confirm=true + return_url — платёж сразу отправляется ученика на оплату,
	// ссылка возвращается в next_action.redirect_to_url.url.
	form.Set("confirm", "true")
	form.Set("return_url", returnURL(req.ReturnURL))
	form.Set("automatic_payment_methods[enabled]", "true")
	form.Set("automatic_payment_methods[allow_redirects]", "true")
	if req.PaymentID != "" {
		form.Set("metadata[internal_payment_id]", req.PaymentID)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBase+"/v1/payment_intents", strings.NewReader(form.Encode()))
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("stripe: не удалось собрать запрос: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.secretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if key := strings.TrimSpace(req.PaymentID); key != "" {
		httpReq.Header.Set("Idempotency-Key", key)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("%w: stripe недоступен: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("%w: stripe: ответ API не прочитан", ErrUpstream)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return CreatePaymentResult{}, fmt.Errorf("%w: stripe вернул %d: %s", ErrUpstream, resp.StatusCode, stripeAPIError(raw))
	}
	var pi stripePaymentIntent
	if err := json.Unmarshal(raw, &pi); err != nil || pi.ID == "" {
		return CreatePaymentResult{}, fmt.Errorf("%w: stripe: некорректный ответ API", ErrUpstream)
	}
	redirect := ""
	if pi.NextAction != nil && pi.NextAction.RedirectToURL != nil {
		redirect = pi.NextAction.RedirectToURL.URL
	}
	// Если платёж сразу завершён (без действия ученика) — ссылка не нужна.
	return CreatePaymentResult{ProviderPaymentID: pi.ID, RedirectURL: redirect}, nil
}

// VerifyWebhook проверяет заголовок Stripe-Signature по сырому телу запроса.
// Формат: t=<unix>,v1=<hex>; подпись = HMAC-SHA256(secret, "<t>.<payload>").
func (s *StripeProvider) VerifyWebhook(r *http.Request, secret []byte) error {
	key := secret
	if len(key) == 0 {
		key = []byte(s.webhookSecret)
	}
	raw, err := readRawBody(r)
	if err != nil {
		return fmt.Errorf("%w: stripe: %v", ErrUnauthorized, err)
	}
	return verifyStripeSignature(r.Header.Get("Stripe-Signature"), raw, key, time.Now(), stripeSignatureTolerance)
}

// verifyStripeSignature — чистая проверка подписи (тестируется без сети).
func verifyStripeSignature(header string, payload, secret []byte, now time.Time, tolerance time.Duration) error {
	if strings.TrimSpace(header) == "" {
		return fmt.Errorf("%w: stripe: заголовок Stripe-Signature отсутствует", ErrUnauthorized)
	}
	if len(secret) == 0 {
		return fmt.Errorf("%w: stripe: не задан STRIPE_WEBHOOK_SECRET", ErrNotConfigured)
	}
	var ts int64 = -1
	var sigs [][]byte
	for _, part := range strings.Split(string(header), ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return fmt.Errorf("%w: stripe: некорректная метка времени в Stripe-Signature", ErrUnauthorized)
			}
			ts = n
		case "v1":
			if b, err := hex.DecodeString(v); err == nil {
				sigs = append(sigs, b)
			}
		}
	}
	if ts < 0 {
		return fmt.Errorf("%w: stripe: нет метки времени t в Stripe-Signature", ErrUnauthorized)
	}
	if len(sigs) == 0 {
		return fmt.Errorf("%w: stripe: нет подписи v1 в Stripe-Signature", ErrUnauthorized)
	}
	// Защита от replay: подпись принимается только в допуске ±tolerance
	// вокруг текущего времени (у Stripe — 300 секунд).
	if d := now.Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return fmt.Errorf("%w: stripe: подпись просрочена (отклонение %s, допуск %s)", ErrUnauthorized, d.Round(time.Second), tolerance)
	}
	// Подпись считаем по сырому телу: "<t>.<payload>".
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(payload)
	expected := mac.Sum(nil)
	// hmac.Equal — сравнение за постоянное время (защита от тайминг-атак).
	// Несколько v1 допускается: Stripe подписывает ротацией секретов.
	for _, sig := range sigs {
		if hmac.Equal(sig, expected) {
			return nil
		}
	}
	return fmt.Errorf("%w: stripe: HMAC-подпись не совпала", ErrUnauthorized)
}

// stripeEvent — минимальная модель вебхук-события.
type stripeEvent struct {
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
		} `json:"object"`
	} `json:"data"`
}

// HandleWebhook проверяет подпись и разбирает событие:
// payment_intent.succeeded → succeeded, payment_intent.payment_failed /
// payment_intent.canceled → failed, charge.refunded → refunded.
func (s *StripeProvider) HandleWebhook(r *http.Request) (WebhookEvent, error) {
	// Подпись проверяем и здесь (повторно — защита в глубину, тело восстановлено).
	if err := s.VerifyWebhook(r, nil); err != nil {
		return WebhookEvent{}, err
	}
	raw, err := readRawBody(r)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("stripe: %v", err)
	}
	var ev stripeEvent
	if err := json.Unmarshal(raw, &ev); err != nil || ev.Data.Object.ID == "" {
		return WebhookEvent{}, errors.New("stripe: некорректное тело вебхука")
	}
	switch ev.Type {
	case "payment_intent.succeeded":
		return WebhookEvent{ProviderPaymentID: ev.Data.Object.ID, Status: "succeeded", Raw: raw}, nil
	case "payment_intent.payment_failed", "payment_intent.canceled":
		return WebhookEvent{ProviderPaymentID: ev.Data.Object.ID, Status: "failed", Raw: raw}, nil
	case "charge.refunded":
		// Возврат приходит по charge; наш внутренний ключ — PaymentIntent.
		id := ev.Data.Object.PaymentIntent
		if id == "" {
			id = ev.Data.Object.ID
		}
		return WebhookEvent{ProviderPaymentID: id, Status: "refunded", Raw: raw}, nil
	default:
		// Прочие события (например, payment_intent.processing) — игнорируем.
		return WebhookEvent{ProviderPaymentID: ev.Data.Object.ID, Status: "ignored", Raw: raw}, nil
	}
}

// Refund создаёт возврат через POST /v1/refunds (полный возврат PaymentIntent).
func (s *StripeProvider) Refund(ctx context.Context, providerPaymentID string, idempotencyKey ...string) error {
	form := url.Values{}
	form.Set("payment_intent", providerPaymentID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBase+"/v1/refunds", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("stripe: не удалось собрать запрос возврата: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.secretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	key := "refund-" + providerPaymentID
	if len(idempotencyKey) > 0 && strings.TrimSpace(idempotencyKey[0]) != "" {
		key = strings.TrimSpace(idempotencyKey[0])
	}
	httpReq.Header.Set("Idempotency-Key", key)
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: stripe недоступен: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%w: stripe вернул %d при возврате: %s", ErrUpstream, resp.StatusCode, stripeAPIError(raw))
	}
	return nil
}

// GetStatus запрашивает статус PaymentIntent: GET /v1/payment_intents/{id}.
func (s *StripeProvider) GetStatus(ctx context.Context, providerPaymentID string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.apiBase+"/v1/payment_intents/"+url.PathEscape(providerPaymentID), nil)
	if err != nil {
		return "", fmt.Errorf("stripe: не удалось собрать запрос статуса: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.secretKey)
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: stripe недоступен: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: stripe вернул %d: %s", ErrUpstream, resp.StatusCode, stripeAPIError(raw))
	}
	var pi stripePaymentIntent
	if err := json.Unmarshal(raw, &pi); err != nil {
		return "", fmt.Errorf("%w: stripe: некорректный ответ API", ErrUpstream)
	}
	return NormalizeStatus(pi.Status), nil
}

// Проверка компиляции: адаптер реализует интерфейс.
var _ PaymentProvider = (*StripeProvider)(nil)
