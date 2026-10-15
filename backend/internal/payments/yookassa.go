package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// YooKassa-адаптер через net/http без SDK.
//   - YOOKASSA_SHOP_ID — идентификатор магазина (Basic-авторизация);
//   - YOOKASSA_SECRET_KEY — секретный ключ;
//   - YOOKASSA_TRUSTED_IPS — опционально: CIDR/IP доверенных источников вебхуков.
//
// Подписи вебхуков у YooKassa НЕТ. Компенсируем тремя механизмами:
//  1. проверка источника по YOOKASSA_TRUSTED_IPS (если задан);
//  2. структурная согласованность события (event.type + идентификатор);
//  3. главное — событие подтверждается запросом к API YooKassa
//     (GET /v3/payments/{id} или /v3/refunds/{id}): подделать статус нельзя,
//     потому что статус берётся от провайдера, а не из тела вебхука.
const (
	// yookassaAPIBase — базовый URL YooKassa v3 (в тестах переопределяется).
	yookassaAPIBase = "https://api.yookassa.ru/v3"
	// yookassaTimeout — таймаут всех исходящих вызовов к YooKassa.
	yookassaTimeout = 15 * time.Second
)

// YookassaProvider — адаптер YooKassa.
type YookassaProvider struct {
	shopID  string
	secret  string
	apiBase string
	client  *http.Client
	trusted []*net.IPNet // пусто — проверка источника выключена (env не задан)
}

// NewYookassaProvider создаёт адаптер YooKassa из env.
func NewYookassaProvider() (*YookassaProvider, error) {
	shopID := strings.TrimSpace(os.Getenv("YOOKASSA_SHOP_ID"))
	secret := strings.TrimSpace(os.Getenv("YOOKASSA_SECRET_KEY"))
	if shopID == "" || secret == "" {
		return nil, fmt.Errorf("%w: yookassa требует YOOKASSA_SHOP_ID и YOOKASSA_SECRET_KEY", ErrNotConfigured)
	}
	p := newYookassaProvider(shopID, secret)
	p.trusted = parseTrustedNets(os.Getenv("YOOKASSA_TRUSTED_IPS"))
	return p, nil
}

// newYookassaProvider — конструктор с явными кредами (используется тестами без env).
func newYookassaProvider(shopID, secret string) *YookassaProvider {
	return &YookassaProvider{
		shopID:  shopID,
		secret:  secret,
		apiBase: yookassaAPIBase,
		client:  &http.Client{Timeout: yookassaTimeout},
	}
}

// Name возвращает имя провайдера.
func (y *YookassaProvider) Name() string { return "yookassa" }

// yookassaAmount — сумма в формате «рубли с двумя знаками».
type yookassaAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// yookassaPayment — минимальная модель платёжного объекта YooKassa.
type yookassaPayment struct {
	ID           string          `json:"id"`
	Status       string          `json:"status"`
	Amount       *yookassaAmount `json:"amount"`
	Confirmation *struct {
		Redirect *struct {
			URL string `json:"url"`
		} `json:"redirect"`
	} `json:"confirmation"`
}

// yookassaValue переводит копейки в строковый формат YooKassa.
// Целочисленное деление вместо float: 4999 коп. → "49.99" без ошибок округления.
func yookassaValue(cents int) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// authorize ставит Basic-авторизацию shopId:secret.
func (y *YookassaProvider) authorize(req *http.Request) {
	req.SetBasicAuth(y.shopID, y.secret)
}

// yookassaAPIError возвращает текст ошибки из ответа API.
func yookassaAPIError(raw []byte) string {
	var e struct {
		Description string `json:"description"`
		Error       string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil {
		if e.Description != "" {
			return e.Description
		}
		if e.Error != "" {
			return e.Error
		}
	}
	return string(raw)
}

// CreatePayment создаёт платёж: POST /v3/payments с Idempotence-Key.
func (y *YookassaProvider) CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResult, error) {
	if req.AmountCents <= 0 {
		return CreatePaymentResult{}, errors.New("yookassa: сумма платежа должна быть больше нуля")
	}
	body := map[string]any{
		"amount": map[string]any{
			"value":    yookassaValue(req.AmountCents),
			"currency": strings.ToUpper(strings.TrimSpace(req.Currency)),
		},
		"capture": true, // сразу списываем: статус сразу succeeded
		"confirmation": map[string]any{
			"type":       "redirect",
			"return_url": returnURL(req.ReturnURL),
		},
		"description": req.Description,
	}
	if req.PaymentID != "" {
		body["metadata"] = map[string]any{"internal_payment_id": req.PaymentID}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("yookassa: не удалось собрать тело: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, y.apiBase+"/payments", strings.NewReader(string(raw)))
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("yookassa: не удалось собрать запрос: %w", err)
	}
	y.authorize(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	// Ключ идемпотентности обязателен для /v3/payments.
	idem := strings.TrimSpace(req.PaymentID)
	if idem == "" {
		idem = uuid.NewString()
	}
	httpReq.Header.Set("Idempotence-Key", idem)

	resp, err := y.client.Do(httpReq)
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("%w: yookassa недоступен: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if err != nil {
		return CreatePaymentResult{}, fmt.Errorf("%w: yookassa: ответ API не прочитан", ErrUpstream)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return CreatePaymentResult{}, fmt.Errorf("%w: yookassa вернул %d: %s", ErrUpstream, resp.StatusCode, yookassaAPIError(respRaw))
	}
	var p yookassaPayment
	if err := json.Unmarshal(respRaw, &p); err != nil || p.ID == "" {
		return CreatePaymentResult{}, fmt.Errorf("%w: yookassa: некорректный ответ API", ErrUpstream)
	}
	redirect := ""
	if p.Confirmation != nil && p.Confirmation.Redirect != nil {
		redirect = p.Confirmation.Redirect.URL
	}
	return CreatePaymentResult{ProviderPaymentID: p.ID, RedirectURL: redirect}, nil
}

// parseTrustedNets разбирает список доверенных адресов/сетей через запятую
// ("1.2.3.4", "10.0.0.0/8", поддержка IPv6).
func parseTrustedNets(raw string) []*net.IPNet {
	var nets []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
				continue
			}
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

// checkSource проверяет, что вебхук пришёл из доверенной сети YOOKASSA_TRUSTED_IPS.
// Если список не задан — проверка пропускается: главная защита — обязательное
// подтверждение события запросом к API YooKassa (см. HandleWebhook).
func (y *YookassaProvider) checkSource(r *http.Request) error {
	if len(y.trusted) == 0 {
		return nil
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: yookassa: не удалось определить IP источника вебхука", ErrUnauthorized)
	}
	for _, n := range y.trusted {
		if n.Contains(ip) {
			return nil
		}
	}
	return fmt.Errorf("%w: yookassa: вебхук из недоверенного IP %s", ErrUnauthorized, ip)
}

// yookassaEvent — модель вебхук-события YooKassa.
type yookassaEvent struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Object struct {
		ID        string          `json:"id"`
		Status    string          `json:"status"`
		PaymentID string          `json:"payment_id"`
		Amount    *yookassaAmount `json:"amount"`
	} `json:"object"`
}

// VerifyWebhook — подпись у YooKassa отсутствует, поэтому проверяем
// источник (YOOKASSA_TRUSTED_IPS) и структурную согласованность события.
// Достоверность события подтверждается в HandleWebhook запросом к API.
func (y *YookassaProvider) VerifyWebhook(r *http.Request, _ []byte) error {
	if err := y.checkSource(r); err != nil {
		return err
	}
	raw, err := readRawBody(r)
	if err != nil {
		return fmt.Errorf("%w: yookassa: %v", ErrUnauthorized, err)
	}
	var ev yookassaEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("%w: yookassa: некорректный JSON вебхука", ErrUnauthorized)
	}
	if !strings.HasPrefix(ev.Type, "payment.") && !strings.HasPrefix(ev.Type, "refund.") {
		return fmt.Errorf("%w: yookassa: неподдерживаемый тип события %q", ErrUnauthorized, ev.Type)
	}
	if ev.Object.ID == "" && ev.Object.PaymentID == "" {
		return fmt.Errorf("%w: yookassa: в событии нет идентификатора платежа", ErrUnauthorized)
	}
	return nil
}

// apiGet выполняет GET-запрос к API YooKassa с Basic-авторизацией.
func (y *YookassaProvider) apiGet(ctx context.Context, path string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, y.apiBase+path, nil)
	if err != nil {
		return nil, fmt.Errorf("yookassa: не удалось собрать запрос: %w", err)
	}
	y.authorize(httpReq)
	resp, err := y.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: yookassa недоступен: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if err != nil {
		return nil, fmt.Errorf("%w: yookassa: ответ API не прочитан", ErrUpstream)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: yookassa вернул %d: %s", ErrUpstream, resp.StatusCode, yookassaAPIError(raw))
	}
	return raw, nil
}

// getPayment подтверждает платёж у YooKassa: GET /v3/payments/{id}.
func (y *YookassaProvider) getPayment(ctx context.Context, id string) (*yookassaPayment, error) {
	raw, err := y.apiGet(ctx, "/payments/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	var p yookassaPayment
	if err := json.Unmarshal(raw, &p); err != nil || p.ID == "" {
		return nil, fmt.Errorf("%w: yookassa: некорректный ответ API", ErrUpstream)
	}
	return &p, nil
}

// HandleWebhook разбирает событие и подтверждает его у YooKassa:
// статус берётся из GET /v3/payments/{id}, а не из тела вебхука —
// фиктивный или подделанный платёж не пройдёт.
func (y *YookassaProvider) HandleWebhook(r *http.Request) (WebhookEvent, error) {
	// Проверка источника/структуры (повторно — тело восстановлено readRawBody).
	if err := y.VerifyWebhook(r, nil); err != nil {
		return WebhookEvent{}, err
	}
	raw, err := readRawBody(r)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("yookassa: %v", err)
	}
	var ev yookassaEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return WebhookEvent{}, errors.New("yookassa: некорректное тело вебхука")
	}
	ctx := r.Context()
	if strings.HasPrefix(ev.Type, "refund.") {
		// Подтверждаем возврат по его собственному id: фиктивный rf_* даст 404.
		if _, err := y.apiGet(ctx, "/refunds/"+url.PathEscape(ev.Object.ID)); err != nil {
			return WebhookEvent{}, fmt.Errorf("yookassa: возврат %s не подтверждён API: %w", ev.Object.ID, err)
		}
		pid := ev.Object.PaymentID
		if pid == "" {
			pid = ev.Object.ID
		}
		return WebhookEvent{ProviderPaymentID: pid, Status: "refunded", Raw: raw}, nil
	}
	// payment.* — подтверждаем платёж и берём статус от провайдера.
	id := ev.Object.ID
	confirmed, err := y.getPayment(ctx, id)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("yookassa: платёж %s не подтверждён API: %w", id, err)
	}
	// Статус из события не используем: только то, что видит сама YooKassa
	// (защита от подделки тела при отсутствии подписи).
	return WebhookEvent{ProviderPaymentID: id, Status: NormalizeStatus(confirmed.Status), Raw: raw}, nil
}

// Refund возвращает платёж полностью: GET платёж → POST /v3/refunds с Idempotence-Key.
func (y *YookassaProvider) Refund(ctx context.Context, providerPaymentID string) error {
	p, err := y.getPayment(ctx, providerPaymentID)
	if err != nil {
		return err
	}
	if p.Amount == nil || p.Amount.Value == "" {
		return fmt.Errorf("yookassa: у платежа %s не удалось определить сумму для возврата", providerPaymentID)
	}
	body, err := json.Marshal(map[string]any{
		"payment_id": providerPaymentID,
		"amount": map[string]any{
			"value":    p.Amount.Value,
			"currency": p.Amount.Currency,
		},
	})
	if err != nil {
		return fmt.Errorf("yookassa: не удалось собрать тело возврата: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, y.apiBase+"/refunds", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("yookassa: не удалось собрать запрос возврата: %w", err)
	}
	y.authorize(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotence-Key", uuid.NewString())
	resp, err := y.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: yookassa недоступен: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%w: yookassa вернул %d при возврате: %s", ErrUpstream, resp.StatusCode, yookassaAPIError(respRaw))
	}
	return nil
}

// GetStatus запрашивает статус платежа: GET /v3/payments/{id}.
func (y *YookassaProvider) GetStatus(ctx context.Context, providerPaymentID string) (string, error) {
	p, err := y.getPayment(ctx, providerPaymentID)
	if err != nil {
		return "", err
	}
	return NormalizeStatus(p.Status), nil
}

// Проверка компиляции: адаптер реализует интерфейс.
var _ PaymentProvider = (*YookassaProvider)(nil)
