# T09 — Провайдеры оплаты: Stripe, YooKassa, подпись вебхуков

**Ключ сессии:** `T09-payments-providers`
**Волна:** 1 · **Слой:** backend (Go)
**Приоритет:** P1 — `converge-report.md:89-91` помечает все адаптеры как стабы

## Проблема

`payments/stripe.go` и `payments/yookassa.go` — четыре метода, каждый возвращает
ошибку «провайдер не настроен в MVP» (`stripe.go:25-43`, `yookassa.go:25-43`).

Дополнительно:

- `POST /payments/webhook/{provider}` (`handler/enroll.go:75` в роутере)
  **не проверяет подпись** — кто угодно может отправить `succeeded` и получить доступ к курсу;
- `payments/provider.go:41-42` — `Refund` и `GetStatus` есть в интерфейсе,
  но ни один HTTP-маршрут их не вызывает, а статус `PaymentRefunded` /
  `EnrollmentRefunded` нигде не выставляется (хотя `spec.md:121` требует возврат);
- `payments/mock.go:30` возвращает `redirectUrl` вида `/pay/mock/{id}`,
  который **никто не обслуживает** — «купить» нельзя даже на mock.

## Владение файлами (жёсткая граница)

```
backend/internal/payments/provider.go
backend/internal/payments/stripe.go
backend/internal/payments/yookassa.go
backend/internal/payments/mock.go
backend/internal/handler/enroll.go
```
`handler/middleware.go` **не трогать** (маршруты — зона T13).
Если нужен новый маршрут — опиши его в отчёте, а не регистрируй.

## Что сделать

### 1. Интерфейс (`provider.go`)

Не ломая обратную совместимость, вынеси проверку подписи в отдельный метод
интерфейса (или в отдельный интерфейс `WebhookVerifier`, чтобы mock её
реализовал):

```go
// VerifyWebhook проверяет подпись/целостность тела запроса провайдера.
VerifyWebhook(r *http.Request, secret []byte) error
```

Если менять интерфейс слишком рискованно — вынеси проверку в
`payments.VerifyWebhook(provider string, r *http.Request) error` (пакетная
функция). Выбери один вариант и обоснуй в отчёте.

### 2. Stripe (`stripe.go`)

Через `net/http` без новых зависимостей (в `go.mod` сейчас только chi, uuid, pgx, crypto —
**не добавляй SDK**, это раздует сборку и нарушит Docker-First принцип конституции).

- `CreatePayment`: `POST https://api.stripe.com/v1/payment_intents`
  с `Authorization: Bearer <STRIPE_SECRET_KEY>`, form-encoded тело,
  idempotency key из контекста; из ответа читать `id`, `status`,
  `next_action.redirect_to_url.url` (или `latest_charge.hosted_invoice_url`).
- `HandleWebhook`: читать **сырое** тело (`io.ReadAll` до парсинга!),
  проверять заголовок `Stripe-Signature`: формат `t=<unix>,v1=<hex>`,
  подпись = `HMAC-SHA256(secret, "<t>.<payload>")`,
  допуск по времени — 300 секунд (защита от replay).
- `Refund`: `POST /v1/refunds` c `payment_intent=<id>`.
- `GetStatus`: `GET /v1/payment_intents/{id}`.
- Конфигурация из env: `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`.
  Без ключей конструктор возвращает ошибку конфигурации —
  **не** молчаливую заглушку.
- Ставки: `ConvertToSmallestUnit` — сумма приходит в копейках (`priceCents`),
  в Stripe это `unit_amount` тоже в минорных единицах. Проверь валюту:
  для `RUB` Stripe ожидает целое число копеек.

### 3. YooKassa (`yookassa.go`)

- `CreatePayment`: `POST https://api.yookassa.ru/v3/payments`
  c `Authorization: Basic base64(shopId:secret)`,
  `Idempotence-Key: <uuid>`, тело `{ amount: { value, currency }, capture: true, confirmation: { return_url } }`.
- `HandleWebhook`: проверять, что событие пришло с доверенного IP либо
  (проще и достаточно для MVP) что в теле `event.type` начинается с
  `payment.` и `payment.status` согласован; **подпись в YooKassa отсутствует** —
  компенсируй rate-limit + реплей-защиту по `Idempotence-Key` и
  обязательной проверке источника. Опиши решение в отчёте.
- `Refund`: `POST /v3/refunds` c `payment_id`.
- `GetStatus`: `GET /v3/payments/{id}`.
- Конфигурация: `YOOKASSA_SHOP_ID`, `YOOKASSA_SECRET_KEY`.
- Формат `value` — «рубли с двумя знаками»: `fmt.Sprintf("%.2f", cents/100)`.

### 4. Mock (`mock.go`)

- Сделай `redirectUrl` рабочим: добавь в `mock.go` экспортируемый
  `func (m *MockProvider) ConfirmPageHandler() http.Handler` либо
  `func MockConfirmPath(paymentID string) string` и опиши в отчёте,
  какой маршрут нужно зарегистрировать в T13 (нужно для E2E-сценария покупки).
- Поддержи состояния: `pending` → `succeeded` по клику, и явная кнопка отказа.

### 5. Хендлер (`enroll.go`)

- В `PaymentWebhook` **до** обработки события вызывай проверку подписи.
  Для `mock` — пропускай (или проверяй общий секрет из
  `MOCK_WEBHOOK_SECRET`, если задан).
- Обрабатывай `refunded`: `SetPaymentStatus(p.ID, repo.PaymentRefunded)` +
  перевод записи в `EnrollmentRefunded` (проверь, есть ли такой метод в
  `StoreAPI`; если нет — **не добавляй его в интерфейс**, это зона T07/T13,
  опиши в отчёте).
- Не проглатывай ошибку `HandleWebhook` — сейчас любой сбой даёт `400`
  с текстом, неотличимым от «плохое тело».

## Критерии приёмки

- [ ] `cd backend && go build ./...` без ошибок
- [ ] `cd backend && go vet ./...` чисто
- [ ] `cd backend && go test ./...` — существующие тесты (в т.ч. идемпотентность
      вебхука и split комиссии) проходят
- [ ] `go.mod` не изменился (или изменился только в сторону убора зависимостей)
- [ ] Вебхук без валидной подписи получает `401`/`400` и **не** выдаёт доступ
- [ ] Подпись Stripe проверяется по **сырому** телу, время допускается ±300 с
- [ ] Есть unit-тесты на проверку подписи (хорошая подпись / плохая / просроченная)
- [ ] Все сетевые вызовы имеют таймаут (`http.Client{Timeout: ...}`)
- [ ] Комментарии и тексты ошибок по-русски

## Проверка

```bash
cd backend
go build ./... && go vet ./... && go test ./...
```

## Отчёт

Перечисли: новые/изменённые сигнатуры интерфейса, список требуемых env-переменных,
какой маршрут нужно добавить в `middleware.go` для mock-страницы оплаты,
и как ты решил проблему отсутствия подписи у YooKassa.
