package payments

// Юнит-тесты проверки подписи вебхуков (T09).
// Тесты не выходят в сеть: сетевые вызовы подменяются httptest-серверами,
// подпись считается локально через crypto/hmac.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stripeHeader собирает валидный заголовок Stripe-Signature для payload.
func stripeHeader(secret string, ts time.Time, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts.Unix())
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func stripeRequest(payload []byte, header string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/payments/webhook/stripe", bytes.NewReader(payload))
	if header != "" {
		r.Header.Set("Stripe-Signature", header)
	}
	return r
}

var testStripePayload = []byte(`{"type":"payment_intent.succeeded","data":{"object":{"id":"pi_123"}}}`)

// Валидная подпись проходит, тело запроса остаётся читаемым повторно.
func TestVerifyStripeSignatureValid(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_test")
	header := stripeHeader("whsec_test", time.Now().Add(-30*time.Second), testStripePayload)
	r := stripeRequest(testStripePayload, header)
	if err := p.VerifyWebhook(r, nil); err != nil {
		t.Fatalf("валидная подпись отклонена: %v", err)
	}
	// Тело должно быть восстановлено для последующего HandleWebhook.
	raw, err := readRawBody(r)
	if err != nil || !bytes.Equal(raw, testStripePayload) {
		t.Fatalf("тело не восстановлено после проверки: err=%v raw=%q", err, raw)
	}
}

// Невалидная подпись: неверный секрет, подделанный payload, отсутствие заголовка.
func TestVerifyStripeSignatureInvalid(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_test")
	cases := []struct {
		name    string
		payload []byte
		header  string
	}{
		{"неверный секрет", testStripePayload, stripeHeader("whsec_other", time.Now(), testStripePayload)},
		{"подделанный payload", []byte(`{"type":"payment.canceled"}`), stripeHeader("whsec_test", time.Now(), testStripePayload)},
		{"нет заголовка", testStripePayload, ""},
		{"мусор вместо заголовка", testStripePayload, "deadbeef"},
		{"нет метки времени", testStripePayload, "v1=00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := stripeRequest(tc.payload, tc.header)
			err := p.VerifyWebhook(r, nil)
			if err == nil {
				t.Fatal("ожидалась ошибка проверки подписи")
			}
			if !strings.Contains(err.Error(), "подпись") && !strings.Contains(err.Error(), "Signature") {
				t.Fatalf("неожиданный текст ошибки: %v", err)
			}
		})
	}
}

// Просроченная подпись (допуск 300 с) и подпись из будущего отклоняются.
func TestVerifyStripeSignatureExpired(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_test")
	old := stripeHeader("whsec_test", time.Now().Add(-301*time.Second), testStripePayload)
	if err := p.VerifyWebhook(stripeRequest(testStripePayload, old), nil); err == nil {
		t.Fatal("просроченная подпись (−301 с) принята")
	}
	fresh := stripeHeader("whsec_test", time.Now().Add(-290*time.Second), testStripePayload)
	if err := p.VerifyWebhook(stripeRequest(testStripePayload, fresh), nil); err != nil {
		t.Fatalf("подпись в пределах допуска (−290 с) отклонена: %v", err)
	}
	future := stripeHeader("whsec_test", time.Now().Add(301*time.Second), testStripePayload)
	if err := p.VerifyWebhook(stripeRequest(testStripePayload, future), nil); err == nil {
		t.Fatal("подпись из будущего (+301 с) принята")
	}
}

// Несколько v1 (ротация секретов): валидный среди них — успех.
func TestVerifyStripeSignatureMultipleV1(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_test")
	valid := stripeHeader("whsec_test", time.Now(), testStripePayload)
	header := valid + ",v1=0000"
	r := stripeRequest(testStripePayload, header)
	if err := p.VerifyWebhook(r, nil); err != nil {
		t.Fatalf("валидная подпись среди нескольких v1 отклонена: %v", err)
	}
}

// HandleWebhook корректно разбирает подписанное событие, в т.ч. возврат.
func TestStripeHandleWebhookParse(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_test")
	header := stripeHeader("whsec_test", time.Now(), testStripePayload)
	ev, err := p.HandleWebhook(stripeRequest(testStripePayload, header))
	if err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if ev.ProviderPaymentID != "pi_123" || ev.Status != "succeeded" {
		t.Fatalf("неверный event: %+v", ev)
	}

	refundPayload := []byte(`{"type":"charge.refunded","data":{"object":{"id":"ch_9","payment_intent":"pi_777"}}}`)
	header = stripeHeader("whsec_test", time.Now(), refundPayload)
	ev, err = p.HandleWebhook(stripeRequest(refundPayload, header))
	if err != nil {
		t.Fatalf("HandleWebhook refund: %v", err)
	}
	if ev.ProviderPaymentID != "pi_777" || ev.Status != "refunded" {
		t.Fatalf("неверный event возврата: %+v", ev)
	}

	// Неподписанный запрос отклоняется до разбора.
	if _, err := p.HandleWebhook(stripeRequest(testStripePayload, "")); err == nil {
		t.Fatal("неподписанный вебхук принят")
	}
}

// Mock: без MOCK_WEBHOOK_SECRET подпись не требуется; с секретом — строго HMAC.
func TestMockVerifyWebhook(t *testing.T) {
	m := NewMockProvider()
	body := []byte(`{"providerPaymentId":"mock_1","status":"succeeded"}`)

	// Без секрета (dev-режим) — пропускаем.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock", bytes.NewReader(body))
	if err := m.VerifyWebhook(r, nil); err != nil {
		t.Fatalf("mock без секрета: %v", err)
	}

	// С явным секретом: неверная/отсутствующая подпись — отказ.
	secret := []byte("mock-secret")
	r = httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock", bytes.NewReader(body))
	if err := m.VerifyWebhook(r, secret); err == nil {
		t.Fatal("webhook без подписи при заданном секрете принят")
	}

	// Валидная подпись — принимается, тело читается повторно.
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	r = httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock", bytes.NewReader(body))
	r.Header.Set("X-Mock-Signature", sig)
	if err := m.VerifyWebhook(r, secret); err != nil {
		t.Fatalf("валидная mock-подпись отклонена: %v", err)
	}
	// Подделанное тело с той же подписью — отказ.
	tampered := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock",
		bytes.NewReader([]byte(`{"providerPaymentId":"mock_2","status":"succeeded"}`)))
	tampered.Header.Set("X-Mock-Signature", sig)
	if err := m.VerifyWebhook(tampered, secret); err == nil {
		t.Fatal("подделанное тело прошло проверку")
	}
}

// Mock: страница подтверждения рендерится и показывает кнопки для pending.
func TestMockConfirmPage(t *testing.T) {
	m := NewMockProvider()
	res, err := m.CreatePayment(t.Context(), CreatePaymentRequest{
		AmountCents: 4900, Currency: "RUB", Description: "Оплата курса: Python",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if !strings.HasPrefix(res.RedirectURL, "/pay/mock/") {
		t.Fatalf("нерабочий redirectUrl: %q", res.RedirectURL)
	}

	r := httptest.NewRequest(http.MethodGet, res.RedirectURL, nil)
	rec := httptest.NewRecorder()
	MockConfirmHandler().ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("страница оплаты: status=%d", rec.Code)
	}
	html := rec.Body.String()
	if !strings.Contains(html, "Оплатить") || !strings.Contains(html, "49.00") {
		t.Fatalf("страница не содержит кнопку/сумму: %s", html)
	}

	// После webhook статус на странице меняется, кнопки скрываются.
	wh := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock",
		strings.NewReader(`{"providerPaymentId":"`+res.ProviderPaymentID+`","status":"succeeded"}`))
	if _, err := m.HandleWebhook(wh); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	rec = httptest.NewRecorder()
	MockConfirmHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, res.RedirectURL, nil))
	html = rec.Body.String()
	if strings.Contains(html, `id="pay"`) || !strings.Contains(html, "succeeded") {
		t.Fatalf("статус не обновился: %s", html)
	}
	if st, err := m.GetStatus(t.Context(), res.ProviderPaymentID); err != nil || st != "succeeded" {
		t.Fatalf("GetStatus=%q err=%v", st, err)
	}
}

// YooKassa: структурная проверка события (подписи у провайдера нет).
func TestYookassaVerifyWebhook(t *testing.T) {
	y := newYookassaProvider("shop", "secret")
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"корректное payment.succeeded", `{"type":"payment.succeeded","object":{"id":"yk_1","status":"succeeded"}}`, false},
		{"корректное refund.succeeded", `{"type":"refund.succeeded","object":{"id":"rf_1","payment_id":"yk_1"}}`, false},
		{"чужой тип события", `{"type":"invoice.paid","object":{"id":"yk_1"}}`, true},
		{"нет идентификатора", `{"type":"payment.succeeded","object":{}}`, true},
		{"мусор вместо JSON", `не json`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(tc.body))
			err := y.VerifyWebhook(r, nil)
			if tc.wantErr && err == nil {
				t.Fatal("ожидался отказ")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("отклонён корректный запрос: %v", err)
			}
			if err != nil && !strings.Contains(err.Error(), "yookassa") {
				t.Fatalf("текст ошибки без контекста провайдера: %v", err)
			}
		})
	}
}

// YooKassa: проверка источника по YOOKASSA_TRUSTED_IPS.
func TestYookassaCheckSource(t *testing.T) {
	y := newYookassaProvider("shop", "secret")
	y.trusted = parseTrustedNets("10.0.0.0/8, 127.0.0.1")

	r := httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", nil)
	r.RemoteAddr = "10.1.2.3:5678"
	if err := y.checkSource(r); err != nil {
		t.Fatalf("доверенная сеть отклонена: %v", err)
	}
	r.RemoteAddr = "8.8.8.8:5678"
	if err := y.checkSource(r); err == nil {
		t.Fatal("недоверенный IP принят")
	}
}

// YooKassa: событие подтверждается запросом к API; статус берётся от провайдера.
func TestYookassaHandleWebhookConfirmed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "shop" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/payments/yk_1":
			fmt.Fprint(w, `{"id":"yk_1","status":"succeeded","amount":{"value":"49.00","currency":"RUB"}}`)
		case "/payments/yk_fake":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"payment-not-found"}`)
		case "/refunds/rf_1":
			fmt.Fprint(w, `{"id":"rf_1","payment_id":"yk_1","status":"succeeded"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	y := newYookassaProvider("shop", "secret")
	y.apiBase = srv.URL

	// Подтверждённый платёж: статус берётся из API, а не из тела.
	body := `{"type":"payment.succeeded","object":{"id":"yk_1","status":"canceled"}}`
	ev, err := y.HandleWebhook(httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body)))
	if err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if ev.ProviderPaymentID != "yk_1" || ev.Status != "succeeded" {
		t.Fatalf("статус должен браться из API: %+v", ev)
	}

	// Возврат подтверждается по /refunds/{id}.
	body = `{"type":"refund.succeeded","object":{"id":"rf_1","payment_id":"yk_1"}}`
	ev, err = y.HandleWebhook(httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body)))
	if err != nil {
		t.Fatalf("HandleWebhook refund: %v", err)
	}
	if ev.ProviderPaymentID != "yk_1" || ev.Status != "refunded" {
		t.Fatalf("неверный event возврата: %+v", ev)
	}

	// Подделанный id не подтверждается API — ErrUpstream, доступ не выдаётся.
	body = `{"type":"payment.succeeded","object":{"id":"yk_fake","status":"succeeded"}}`
	_, err = y.HandleWebhook(httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body)))
	if err == nil {
		t.Fatal("неподтверждённый платёж прошёл обработку")
	}
}

// YooKassa: CreatePayment шлёт Basic-авторизацию, Idempotence-Key и формат суммы.
func TestYookassaCreatePayment(t *testing.T) {
	var gotAuth, gotIdem string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("Idempotence-Key")
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		gotBody = buf.String()
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":"yk_new","status":"pending","confirmation":{"redirect":{"url":"https://yookassa/pay"}}}`)
	}))
	defer srv.Close()

	y := newYookassaProvider("shop", "secret")
	y.apiBase = srv.URL
	res, err := y.CreatePayment(t.Context(), CreatePaymentRequest{
		PaymentID: "idem-1", AmountCents: 4999, Currency: "RUB", Description: "Курс",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if res.ProviderPaymentID != "yk_new" || res.RedirectURL != "https://yookassa/pay" {
		t.Fatalf("неверный результат: %+v", res)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("нет Basic-авторизации: %q", gotAuth)
	}
	if gotIdem != "idem-1" {
		t.Fatalf("Idempotence-Key=%q", gotIdem)
	}
	if !strings.Contains(gotBody, `"value":"49.99"`) || !strings.Contains(gotBody, `"currency":"RUB"`) {
		t.Fatalf("сумма в неверном формате: %s", gotBody)
	}
}

// Stripe: CreatePayment шлёт Bearer-токен, копейки и Idempotency-Key.
func TestStripeCreatePayment(t *testing.T) {
	var gotAuth, gotIdem string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("Idempotency-Key")
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		gotBody = buf.String()
		fmt.Fprint(w, `{"id":"pi_new","status":"requires_action","next_action":{"redirect_to_url":{"url":"https://stripe/pay"}}}`)
	}))
	defer srv.Close()

	s := newStripeProvider("sk_test", "whsec_test")
	s.apiBase = srv.URL
	res, err := s.CreatePayment(t.Context(), CreatePaymentRequest{
		PaymentID: "idem-2", AmountCents: 4900, Currency: "RUB", Description: "Курс",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if res.ProviderPaymentID != "pi_new" || res.RedirectURL != "https://stripe/pay" {
		t.Fatalf("неверный результат: %+v", res)
	}
	if gotAuth != "Bearer sk_test" {
		t.Fatalf("Authorization=%q", gotAuth)
	}
	if gotIdem != "idem-2" {
		t.Fatalf("Idempotency-Key=%q", gotIdem)
	}
	if !strings.Contains(gotBody, "amount=4900") || !strings.Contains(gotBody, "currency=rub") {
		t.Fatalf("сумма/валюта в неверном формате: %s", gotBody)
	}
}

// NormalizeStatus: refunded не должен схлопываться в pending.
func TestNormalizeStatusRefunded(t *testing.T) {
	for _, raw := range []string{"refunded", "payment.refunded", "REFUNDED"} {
		if got := NormalizeStatus(raw); got != "refunded" {
			t.Fatalf("NormalizeStatus(%q)=%q, ожидался refunded", raw, got)
		}
	}
}

// Фабрика: stripe/yookassa без ключей возвращают ErrNotConfigured, а не заглушку.
func TestNewProviderNotConfigured(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")
	t.Setenv("YOOKASSA_SHOP_ID", "")
	t.Setenv("YOOKASSA_SECRET_KEY", "")
	for _, name := range []string{"stripe", "yookassa"} {
		if _, err := NewProvider(name); err == nil {
			t.Fatalf("NewProvider(%q) без ключей не вернул ошибку", name)
		}
	}
	if _, err := NewProvider("unknown"); err == nil {
		t.Fatal("неизвестный провайдер не вернул ошибку")
	}
	if p, err := NewProvider("mock"); err != nil || p.Name() != "mock" {
		t.Fatalf("mock-провайдер недоступен: %v", err)
	}
}
