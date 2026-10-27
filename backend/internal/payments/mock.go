package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// MockProvider — рабочая in-memory реализация для MVP и тестов.
//
// Реестр платежей общий для всех экземпляров (package-level): каждый вызов
// NewProvider создаёт новый экземпляр, а вебхук и страница подтверждения
// живут вне CreateEnrollment. Источник истины — хранилище Store;
// реестр хранит только состояние страницы оплаты для GetStatus/Refund.
type mockPayment struct {
	Status      string
	AmountCents int
	Currency    string
	Description string
}

var mockRegistry = struct {
	sync.RWMutex
	items map[string]*mockPayment
}{items: make(map[string]*mockPayment)}

// MockPaymentIDPrefix — префикс providerPaymentId, который выдаёт mock.
// Экспортирован, чтобы handler возврата мог отличить mock-платёж от
// бесплатного (free-...) и не зависеть от in-memory реестра после рестарта.
const MockPaymentIDPrefix = "mock_"

// MockProvider — mock-провайдер.
type MockProvider struct {
	// dev разрешает работу без MOCK_WEBHOOK_SECRET и вообще существование
	// mock-провайдера. Вне dev mock запрещён (см. NewProvider).
	dev bool
}

// NewMockProvider создаёт mock-провайдер. Без явного dev-флага сохраняется
// историческое поведение (dev): handler всегда передаёт режим из Config и
// запрещает mock вне dev. Явный false делает mock непригодным и для
// CreatePayment, и для unsigned-вебхука.
func NewMockProvider(dev ...bool) *MockProvider {
	devMode := true
	if len(dev) > 0 {
		devMode = dev[0]
	}
	return &MockProvider{dev: devMode}
}

// Name возвращает имя провайдера.
func (m *MockProvider) Name() string { return "mock" }

// MockConfirmPath — путь страницы подтверждения mock-платежа.
// Регистрируется в роутере зоной T13: r.Get("/pay/mock/{id}", payments.MockConfirmHandler())
func MockConfirmPath(paymentID string) string {
	return "/pay/mock/" + url.PathEscape(paymentID)
}

// CreatePayment создаёт mock-платёж и ссылку на рабочую страницу оплаты.
func (m *MockProvider) CreatePayment(_ context.Context, req CreatePaymentRequest) (CreatePaymentResult, error) {
	if !m.dev {
		return CreatePaymentResult{}, fmt.Errorf("%w: mock-провайдер запрещён вне dev", ErrNotConfigured)
	}
	if req.AmountCents < 0 {
		return CreatePaymentResult{}, errors.New("mock: отрицательная сумма платежа")
	}
	id := MockPaymentIDPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	mockRegistry.Lock()
	mockRegistry.items[id] = &mockPayment{
		Status:      "pending",
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
		Description: req.Description,
	}
	mockRegistry.Unlock()
	return CreatePaymentResult{
		ProviderPaymentID: id,
		RedirectURL:       MockConfirmPath(id),
	}, nil
}

type mockWebhookBody struct {
	ProviderPaymentID string `json:"providerPaymentId"`
	Status            string `json:"status"`
}

// VerifyWebhook: у mock нет внешней подписи. Если задан MOCK_WEBHOOK_SECRET
// (или передан secret явно), требуем заголовок X-Mock-Signature —
// hex HMAC-SHA256 по сырому телу. Без секрета — dev-режим, подпись не нужен.
func (m *MockProvider) VerifyWebhook(r *http.Request, secret []byte) error {
	key := secret
	if len(key) == 0 {
		if v := strings.TrimSpace(os.Getenv("MOCK_WEBHOOK_SECRET")); v != "" {
			key = []byte(v)
		}
	}
	if len(key) == 0 {
		// Пустой MOCK_WEBHOOK_SECRET допустим только в dev: вне dev mock-
		// вебхук без подписи принимать нельзя (защита в глубину — сам
		// провайдер вне dev уже не создаётся).
		if m.dev {
			return nil
		}
		return fmt.Errorf("%w: mock: MOCK_WEBHOOK_SECRET не задан вне dev", ErrUnauthorized)
	}
	raw, err := readRawBody(r)
	if err != nil {
		return fmt.Errorf("%w: mock: %v", ErrUnauthorized, err)
	}
	got := strings.TrimSpace(r.Header.Get("X-Mock-Signature"))
	if got == "" {
		return fmt.Errorf("%w: mock: отсутствует заголовок X-Mock-Signature", ErrUnauthorized)
	}
	got = strings.TrimPrefix(got, "sha256=")
	sig, err := hex.DecodeString(got)
	if err != nil {
		return fmt.Errorf("%w: mock: подпись не является hex", ErrUnauthorized)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return fmt.Errorf("%w: mock: HMAC-подпись не совпала", ErrUnauthorized)
	}
	return nil
}

// HandleWebhook разбирает JSON webhook: {"providerPaymentId","status"}
// и синхронизирует состояние реестра (страница оплаты показывает итог).
func (m *MockProvider) HandleWebhook(r *http.Request) (WebhookEvent, error) {
	raw, err := readRawBody(r)
	if err != nil {
		return WebhookEvent{}, errors.New("mock: " + err.Error())
	}
	var body mockWebhookBody
	if err := json.Unmarshal(raw, &body); err != nil || body.ProviderPaymentID == "" {
		return WebhookEvent{}, errors.New("mock: некорректный webhook (нужны providerPaymentId и status)")
	}
	status := NormalizeStatus(body.Status)
	mockRegistry.Lock()
	if p, ok := mockRegistry.items[body.ProviderPaymentID]; ok {
		p.Status = status
	}
	mockRegistry.Unlock()
	return WebhookEvent{
		ProviderPaymentID: body.ProviderPaymentID,
		Status:            status,
		Raw:               raw,
	}, nil
}

// Refund помечает mock-платёж возвращённым.
func (m *MockProvider) Refund(_ context.Context, providerPaymentID string, _ ...string) error {
	mockRegistry.Lock()
	defer mockRegistry.Unlock()
	p, ok := mockRegistry.items[providerPaymentID]
	if !ok {
		return errors.New("mock: платёж не найден")
	}
	p.Status = "refunded"
	return nil
}

// GetStatus возвращает статус mock-платежа.
func (m *MockProvider) GetStatus(_ context.Context, providerPaymentID string) (string, error) {
	mockRegistry.RLock()
	defer mockRegistry.RUnlock()
	p, ok := mockRegistry.items[providerPaymentID]
	if !ok {
		return "", errors.New("mock: платёж не найден")
	}
	return p.Status, nil
}

// ---- Страница подтверждения mock-платежа ----

// mockConfirmHTML — минимальная страница «оплаты» для mock-провайдера.
// Кнопки шлют POST /api/v1/payments/webhook/mock, поэтому запись курса
// активируется тем же кодом, что и для реальных провайдеров.
const mockConfirmHTML = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Тестовая оплата — mock</title>
<style>
body{font-family:system-ui,sans-serif;background:#f4f5f7;margin:0;display:flex;justify-content:center;padding-top:8vh}
main{background:#fff;border-radius:12px;padding:32px;max-width:480px;width:100%;box-shadow:0 2px 12px rgba(0,0,0,.08)}
h1{font-size:20px;margin-top:0}
code{background:#f0f1f3;padding:2px 6px;border-radius:4px}
button{font-size:16px;padding:10px 20px;border-radius:8px;border:0;cursor:pointer;margin-right:8px}
button.primary{background:#2563eb;color:#fff}
button.secondary{background:#e5e7eb}
pre{background:#f0f1f3;padding:12px;border-radius:8px;white-space:pre-wrap;margin-top:16px}
.muted{color:#6b7280}
</style>
</head>
<body>
<main>
<h1>Тестовая оплата (mock)</h1>
{{if not .Found}}
<p>Платёж не найден.</p>
{{else}}
<p>Платёж <code>{{.ID}}</code>{{if .Description}} — {{.Description}}{{end}}</p>
<p>Сумма: <strong>{{.Amount}} {{.Currency}}</strong></p>
<p>Статус: <strong id="status">{{.Status}}</strong></p>
{{if eq .Status "pending"}}
<p>
  <button id="pay" class="primary">Оплатить</button>
  <button id="fail" class="secondary">Отказаться от оплаты</button>
</p>
{{else}}
<p class="muted">Платёж уже обработан. Вернитесь в приложение и обновите раздел «Мои курсы».</p>
{{end}}
<pre id="result"></pre>
<script>
(function () {
  function post(status, btn) {
    fetch("/api/v1/payments/webhook/mock", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify({providerPaymentId: {{.IDJSON}}, status: status})
    }).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (data) {
        document.getElementById("status").textContent = status;
        document.getElementById("result").textContent = res.status + " — " + JSON.stringify(data);
        if (btn) { btn.disabled = true; }
      });
    }).catch(function (e) {
      document.getElementById("result").textContent = "Ошибка запроса: " + e;
    });
  }
  var pay = document.getElementById("pay");
  if (pay) { pay.addEventListener("click", function () { post("succeeded", pay); }); }
  var fail = document.getElementById("fail");
  if (fail) { fail.addEventListener("click", function () { post("failed", fail); }); }
})();
</script>
{{end}}
</main>
</body>
</html>`

var mockConfirmTmpl = template.Must(template.New("mockConfirm").Parse(mockConfirmHTML))

// MockConfirmData — данные шаблона страницы оплаты. Экспортирован, чтобы
// handler мог собрать страницу из долговечного Store (после рестарта API
// in-memory реестр провайдера пуст, а платёж в БД остаётся).
type MockConfirmData struct {
	ID          string
	IDJSON      template.JS // id внутри JS-строки (html/template экранирует контекст)
	Amount      string
	Currency    string
	Description string
	Status      string
	Found       bool
}

// RenderMockConfirm отдаёт HTML-страницу подтверждения с готовыми данными.
func RenderMockConfirm(w http.ResponseWriter, data MockConfirmData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := mockConfirmTmpl.Execute(w, data); err != nil {
		http.Error(w, "Не удалось отобразить страницу оплаты", http.StatusInternalServerError)
	}
}

// MockConfirmHandler — HTML-страница подтверждения mock-платежа из
// in-memory реестра провайдера. Оставлен для совместимости; рабочий маршрут
// /pay/mock/{id} обслуживает handler.App.MockPaymentPage, который читает
// платёж из Store и переживает перезапуск (важно для Postgres).
// ID платежа берётся из последнего сегмента пути, поэтому handler не зависит
// от chi и параметров роутинга.
func MockConfirmHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := pathBase(r.URL.Path)
		mockRegistry.RLock()
		p := mockRegistry.items[id]
		var data MockConfirmData
		if p != nil {
			data = MockConfirmData{
				ID:          id,
				IDJSON:      template.JS(strconvQuote(id)),
				Amount:      fmt.Sprintf("%d.%02d", p.AmountCents/100, p.AmountCents%100),
				Currency:    p.Currency,
				Description: p.Description,
				Status:      p.Status,
				Found:       true,
			}
		}
		mockRegistry.RUnlock()
		RenderMockConfirm(w, data)
	})
}

// pathBase возвращает последний сегмент пути без зависимостей от net/url-семантики.
func pathBase(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// strconvQuote — минимальный экранированный JSON-стринг для JS-контекста.
// Платёжные id имеют безопасный формат (mock_<hex>), но экранируем всё равно.
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Проверка компиляции: адаптер реализует интерфейс.
var _ PaymentProvider = (*MockProvider)(nil)
