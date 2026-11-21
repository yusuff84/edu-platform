package payments

// Контрактные тесты HTTP-адаптеров Stripe и YooKassa (T09/T13).
// Все исходящие вызовы идут на httptest.Server через подмену apiBase —
// ни один тест не обращается к api.stripe.com или api.yookassa.ru.
//
// Покрыты ветки, которых не было в webhook_test.go:
//   - CreatePayment: успех (200/201, наличие/отсутствие redirect),
//     4xx/5xx, некорректный JSON, недоступность и таймаут;
//   - GetStatus: маппинг статусов провайдера и ошибки API;
//   - Refund: тело/заголовки запроса, успех и ошибки API;
//   - HandleWebhook/VerifyWebhook: непокрытые события и защитные ветки.

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
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// requireErrIs проверяет, что err оборачивает нужный sentinel и содержит
// ожидаемый фрагмент текста (пустая строка — без проверки текста).
func requireErrIs(t *testing.T, err error, target error, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка %v, получено nil", target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("ошибка не оборачивает %v: %v", target, err)
	}
	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Fatalf("в ошибке нет %q: %v", contains, err)
	}
}

// mockHMACHex — подпись mock-провайдера: hex(HMAC-SHA256(secret, raw)).
func mockHMACHex(secret, raw []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

// requireErrContains проверяет обычную (не sentinel) ошибку по тексту.
func requireErrContains(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка с текстом %q, получено nil", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("в ошибке нет %q: %v", contains, err)
	}
}

// closedServerURL возвращает URL уже остановленного httptest-сервера:
// обращение к нему детерминированно даёт connection refused без сети.
func closedServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	return srv.URL
}

// hangingServer — httptest-сервер, который не отвечает до конца теста:
// запрос детерминированно упирается в context deadline. Cleanup сначала
// отпускает handler (LIFO), затем закрывает сервер, поэтому Close не висит.
func hangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

// ---------- Stripe: CreatePayment ----------

func TestStripeCreatePaymentResponseHandling(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantID       string
		wantRedirect string
	}{
		{
			name:   "requires_action с redirect",
			status: http.StatusOK,
			body: `{"id":"pi_1","status":"requires_action",
				"next_action":{"redirect_to_url":{"url":"https://checkout.stripe.test/pi_1"}}}`,
			wantID:       "pi_1",
			wantRedirect: "https://checkout.stripe.test/pi_1",
		},
		{
			name:   "succeeded без next_action",
			status: http.StatusCreated,
			body:   `{"id":"pi_2","status":"succeeded"}`,
			wantID: "pi_2",
		},
		{
			name:   "requires_action без redirect",
			status: http.StatusOK,
			body:   `{"id":"pi_3","status":"requires_action","next_action":{}}`,
			wantID: "pi_3",
		},
		{
			name:   "next_action null",
			status: http.StatusOK,
			body:   `{"id":"pi_4","status":"requires_action","next_action":null}`,
			wantID: "pi_4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			s := newStripeProvider("sk_test", "whsec_test")
			s.apiBase = srv.URL
			res, err := s.CreatePayment(t.Context(), CreatePaymentRequest{
				PaymentID: "idem-1", AmountCents: 4900, Currency: "RUB", Description: "Курс",
			})
			if err != nil {
				t.Fatalf("CreatePayment: %v", err)
			}
			if gotMethod != http.MethodPost || gotPath != "/v1/payment_intents" {
				t.Fatalf("запрос %s %s, ждали POST /v1/payment_intents", gotMethod, gotPath)
			}
			if res.ProviderPaymentID != tc.wantID || res.RedirectURL != tc.wantRedirect {
				t.Fatalf("результат %+v, ждали id=%q redirect=%q", res, tc.wantID, tc.wantRedirect)
			}
		})
	}
}

func TestStripeCreatePaymentAPIErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "400 с сообщением Stripe",
			status:  http.StatusBadRequest,
			body:    `{"error":{"message":"Your card was declined."}}`,
			wantSub: "Your card was declined.",
		},
		{
			name:    "401 неверный ключ",
			status:  http.StatusUnauthorized,
			body:    `{"error":{"message":"Invalid API Key provided"}}`,
			wantSub: "Invalid API Key provided",
		},
		{
			name:    "429 rate limit",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"Too many requests"}}`,
			wantSub: "Too many requests",
		},
		{
			name:    "500 без JSON",
			status:  http.StatusInternalServerError,
			body:    `upstream exploded`,
			wantSub: "upstream exploded",
		},
		{
			name:    "200 с мусором вместо JSON",
			status:  http.StatusOK,
			body:    `not-json`,
			wantSub: "некорректный ответ API",
		},
		{
			name:    "200 без id",
			status:  http.StatusOK,
			body:    `{"status":"requires_action"}`,
			wantSub: "некорректный ответ API",
		},
		{
			name:    "200 c пустым id",
			status:  http.StatusOK,
			body:    `{"id":"","status":"requires_action"}`,
			wantSub: "некорректный ответ API",
		},
		{
			name:    "200 c JSON-массивом",
			status:  http.StatusOK,
			body:    `[]`,
			wantSub: "некорректный ответ API",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			s := newStripeProvider("sk_test", "whsec_test")
			s.apiBase = srv.URL
			_, err := s.CreatePayment(t.Context(), CreatePaymentRequest{
				PaymentID: "idem-1", AmountCents: 4900, Currency: "RUB",
			})
			requireErrIs(t, err, ErrUpstream, tc.wantSub)
		})
	}
}

func TestStripeCreatePaymentValidatesAmount(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newStripeProvider("sk_test", "whsec_test")
	s.apiBase = srv.URL
	for _, amount := range []int{0, -100} {
		_, err := s.CreatePayment(t.Context(), CreatePaymentRequest{AmountCents: amount, Currency: "RUB"})
		requireErrContains(t, err, "больше нуля")
		if errors.Is(err, ErrUpstream) {
			t.Fatalf("локальная валидация не должна быть ErrUpstream: %v", err)
		}
	}
	if hit {
		t.Fatal("при некорректной сумме запрос к API не должен выполняться")
	}
}

func TestStripeCreatePaymentUnavailable(t *testing.T) {
	s := newStripeProvider("sk_test", "whsec_test")
	s.apiBase = closedServerURL(t)
	_, err := s.CreatePayment(t.Context(), CreatePaymentRequest{
		PaymentID: "idem-1", AmountCents: 4900, Currency: "RUB",
	})
	requireErrIs(t, err, ErrUpstream, "stripe недоступен")
}

func TestStripeCreatePaymentTimeout(t *testing.T) {
	s := newStripeProvider("sk_test", "whsec_test")
	s.apiBase = hangingServer(t).URL

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := s.CreatePayment(ctx, CreatePaymentRequest{
		PaymentID: "idem-1", AmountCents: 4900, Currency: "RUB",
	})
	requireErrIs(t, err, ErrUpstream, "stripe недоступен")
}

// ---------- Stripe: GetStatus ----------

func TestStripeGetStatusMapping(t *testing.T) {
	cases := []struct {
		providerStatus string
		want           string
	}{
		{"", "pending"},
		{"succeeded", "succeeded"},
		{"requires_action", "pending"},
		{"requires_payment_method", "pending"},
		{"processing", "pending"},
		{"canceled", "failed"},
		{"unknown_future_status", "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.providerStatus, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
				fmt.Fprintf(w, `{"id":"pi_42","status":%q}`, tc.providerStatus)
			}))
			defer srv.Close()

			s := newStripeProvider("sk_test", "whsec_test")
			s.apiBase = srv.URL
			got, err := s.GetStatus(t.Context(), "pi_42")
			if err != nil {
				t.Fatalf("GetStatus: %v", err)
			}
			if got != tc.want {
				t.Fatalf("GetStatus(%q)=%q, ждали %q", tc.providerStatus, got, tc.want)
			}
			if gotMethod != http.MethodGet || gotPath != "/v1/payment_intents/pi_42" {
				t.Fatalf("запрос %s %s, ждали GET /v1/payment_intents/pi_42", gotMethod, gotPath)
			}
			if gotAuth != "Bearer sk_test" {
				t.Fatalf("Authorization=%q", gotAuth)
			}
		})
	}
}

func TestStripeGetStatusAPIErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "404 с сообщением",
			status:  http.StatusNotFound,
			body:    `{"error":{"message":"No such payment_intent"}}`,
			wantSub: "No such payment_intent",
		},
		{
			name:    "500 без JSON",
			status:  http.StatusInternalServerError,
			body:    `boom`,
			wantSub: "boom",
		},
		{
			name:    "200 с некорректным JSON",
			status:  http.StatusOK,
			body:    `{"id":`,
			wantSub: "некорректный ответ API",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			s := newStripeProvider("sk_test", "whsec_test")
			s.apiBase = srv.URL
			_, err := s.GetStatus(t.Context(), "pi_404")
			requireErrIs(t, err, ErrUpstream, tc.wantSub)
		})
	}
}

// ---------- Stripe: Refund ----------

func TestStripeRefundSuccess(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth, gotIdem, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				gotIdem = r.Header.Get("Idempotency-Key")
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"id":"re_1","status":"succeeded"}`)
			}))
			defer srv.Close()

			s := newStripeProvider("sk_test", "whsec_test")
			s.apiBase = srv.URL
			if err := s.Refund(t.Context(), "pi_1"); err != nil {
				t.Fatalf("Refund: %v", err)
			}
			if gotMethod != http.MethodPost || gotPath != "/v1/refunds" {
				t.Fatalf("запрос %s %s, ждали POST /v1/refunds", gotMethod, gotPath)
			}
			if gotAuth != "Bearer sk_test" {
				t.Fatalf("Authorization=%q", gotAuth)
			}
			if gotIdem != "refund-pi_1" {
				t.Fatalf("Idempotency-Key=%q, ждали refund-pi_1", gotIdem)
			}
			form, err := url.ParseQuery(gotBody)
			if err != nil {
				t.Fatalf("тело возврата не form-urlencoded: %v (%q)", err, gotBody)
			}
			if form.Get("payment_intent") != "pi_1" {
				t.Fatalf("payment_intent=%q, тело=%q", form.Get("payment_intent"), gotBody)
			}
		})
	}
}

func TestStripeRefundAPIError(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "400 с сообщением",
			status:  http.StatusBadRequest,
			body:    `{"error":{"message":"Charge has already been refunded."}}`,
			wantSub: "Charge has already been refunded.",
		},
		{
			name:    "500 без JSON",
			status:  http.StatusInternalServerError,
			body:    `refund failed`,
			wantSub: "refund failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			s := newStripeProvider("sk_test", "whsec_test")
			s.apiBase = srv.URL
			err := s.Refund(t.Context(), "pi_1")
			requireErrIs(t, err, ErrUpstream, tc.wantSub)
		})
	}
}

// ---------- Stripe: вебхуки (непокрытые ветки) ----------

func TestStripeHandleWebhookUncoveredEvents(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_test")
	cases := []struct {
		name       string
		payload    string
		wantID     string
		wantStatus string
		wantErr    bool
	}{
		{
			name:       "payment_failed → failed",
			payload:    `{"type":"payment_intent.payment_failed","data":{"object":{"id":"pi_fail"}}}`,
			wantID:     "pi_fail",
			wantStatus: "failed",
		},
		{
			name:       "payment_intent.canceled → failed",
			payload:    `{"type":"payment_intent.canceled","data":{"object":{"id":"pi_cancel"}}}`,
			wantID:     "pi_cancel",
			wantStatus: "failed",
		},
		{
			name:       "charge.refunded без payment_intent → id charge",
			payload:    `{"type":"charge.refunded","data":{"object":{"id":"ch_only"}}}`,
			wantID:     "ch_only",
			wantStatus: "refunded",
		},
		{
			name:       "processing игнорируется",
			payload:    `{"type":"payment_intent.processing","data":{"object":{"id":"pi_proc"}}}`,
			wantID:     "pi_proc",
			wantStatus: "ignored",
		},
		{
			name:       "неизвестный тип игнорируется",
			payload:    `{"type":"customer.created","data":{"object":{"id":"cus_1"}}}`,
			wantID:     "cus_1",
			wantStatus: "ignored",
		},
		{
			name:    "подписанный мусорный JSON",
			payload: `не json`,
			wantErr: true,
		},
		{
			name:    "событие без id объекта",
			payload: `{"type":"payment_intent.succeeded","data":{"object":{}}}`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.payload)
			header := stripeHeader("whsec_test", time.Now(), payload)
			ev, err := p.HandleWebhook(stripeRequest(payload, header))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ожидалась ошибка, событие: %+v", ev)
				}
				return
			}
			if err != nil {
				t.Fatalf("HandleWebhook: %v", err)
			}
			if ev.ProviderPaymentID != tc.wantID || ev.Status != tc.wantStatus {
				t.Fatalf("событие %+v, ждали id=%q status=%q", ev, tc.wantID, tc.wantStatus)
			}
			if string(ev.Raw) != tc.payload {
				t.Fatalf("Raw не равен исходному телу: %q", ev.Raw)
			}
		})
	}
}

func TestStripeVerifyWebhookExplicitSecret(t *testing.T) {
	p := newStripeProvider("sk_test", "whsec_env")
	payload := testStripePayload

	// Явный секрет перекрывает секрет провайдера (ветка key := secret).
	header := stripeHeader("whsec_explicit", time.Now(), payload)
	if err := p.VerifyWebhook(stripeRequest(payload, header), []byte("whsec_explicit")); err != nil {
		t.Fatalf("явный секрет отклонён: %v", err)
	}
	// Секрет провайдера при явной подмене не должен приниматься.
	if err := p.VerifyWebhook(stripeRequest(payload, header), []byte("whsec_other")); err == nil {
		t.Fatal("подпись чужим явным секретом принята")
	}
}

func TestVerifyStripeSignatureMalformedFields(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name   string
		header string
	}{
		{"нечисловая метка времени", fmt.Sprintf("t=abc,v1=%s", strings.Repeat("00", 32))},
		{"нет v1", fmt.Sprintf("t=%d", now)},
		{"v1 не hex", fmt.Sprintf("t=%d,v1=zzzz", now)},
		{"пустой заголовок", " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyStripeSignature(tc.header, testStripePayload, []byte("whsec_test"), time.Unix(now, 0), 300*time.Second)
			requireErrIs(t, err, ErrUnauthorized, "")
		})
	}

	// Пустой секрет — ошибка конфигурации, а не «подпись неверна».
	err := verifyStripeSignature(fmt.Sprintf("t=%d,v1=%s", now, strings.Repeat("00", 32)),
		testStripePayload, nil, time.Unix(now, 0), 300*time.Second)
	requireErrIs(t, err, ErrNotConfigured, "STRIPE_WEBHOOK_SECRET")
}

// ---------- YooKassa: CreatePayment ----------

func TestYookassaCreatePaymentWithoutPaymentID(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotIdem, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		user, pass, ok := r.BasicAuth()
		if ok {
			gotAuth = user + ":" + pass
		}
		gotIdem = r.Header.Get("Idempotence-Key")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusOK) // YooKassa отвечает 200, а не только 201
		_, _ = io.WriteString(w, `{"id":"yk_new","status":"pending"}`)
	}))
	defer srv.Close()

	y := newYookassaProvider("shop", "secret")
	y.apiBase = srv.URL
	res, err := y.CreatePayment(t.Context(), CreatePaymentRequest{
		AmountCents: 1000, Currency: " rub ", Description: "Курс",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/payments" {
		t.Fatalf("запрос %s %s, ждали POST /payments", gotMethod, gotPath)
	}
	if gotAuth != "shop:secret" {
		t.Fatalf("Basic-авторизация=%q, ждали shop:secret", gotAuth)
	}
	if _, err := uuid.Parse(gotIdem); err != nil {
		t.Fatalf("Idempotence-Key должен быть сгенерирован как UUID: %q (%v)", gotIdem, err)
	}
	if res.ProviderPaymentID != "yk_new" || res.RedirectURL != "" {
		t.Fatalf("результат %+v, ждали id=yk_new без redirect", res)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("тело не JSON: %v (%q)", err, gotBody)
	}
	amount, _ := body["amount"].(map[string]any)
	if amount["value"] != "10.00" || amount["currency"] != "RUB" {
		t.Fatalf("amount=%v, ждали 10.00 RUB", body["amount"])
	}
	if body["capture"] != true {
		t.Fatalf("capture=%v, ждали true", body["capture"])
	}
	confirmation, _ := body["confirmation"].(map[string]any)
	if confirmation["type"] != "redirect" {
		t.Fatalf("confirmation=%v, ждали type=redirect", body["confirmation"])
	}
	if _, hasMetadata := body["metadata"]; hasMetadata {
		t.Fatalf("metadata без PaymentID не должна отправляться: %v", body["metadata"])
	}
}

func TestYookassaCreatePaymentAPIErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "400 с description",
			status:  http.StatusBadRequest,
			body:    `{"type":"error","description":"Invalid amount","code":"invalid_request"}`,
			wantSub: "Invalid amount",
		},
		{
			name:    "401 с error",
			status:  http.StatusUnauthorized,
			body:    `{"type":"error","error":"invalid_credentials"}`,
			wantSub: "invalid_credentials",
		},
		{
			name:    "500 без JSON",
			status:  http.StatusInternalServerError,
			body:    `internal error`,
			wantSub: "internal error",
		},
		{
			name:    "200 с мусором вместо JSON",
			status:  http.StatusOK,
			body:    `not-json`,
			wantSub: "некорректный ответ API",
		},
		{
			name:    "200 без id",
			status:  http.StatusOK,
			body:    `{"status":"pending"}`,
			wantSub: "некорректный ответ API",
		},
		{
			name:    "201 c пустым id",
			status:  http.StatusCreated,
			body:    `{"id":"","status":"pending"}`,
			wantSub: "некорректный ответ API",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			y := newYookassaProvider("shop", "secret")
			y.apiBase = srv.URL
			_, err := y.CreatePayment(t.Context(), CreatePaymentRequest{
				PaymentID: "idem-1", AmountCents: 4999, Currency: "RUB",
			})
			requireErrIs(t, err, ErrUpstream, tc.wantSub)
		})
	}
}

func TestYookassaCreatePaymentValidatesAmount(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	y := newYookassaProvider("shop", "secret")
	y.apiBase = srv.URL
	for _, amount := range []int{0, -100} {
		_, err := y.CreatePayment(t.Context(), CreatePaymentRequest{AmountCents: amount, Currency: "RUB"})
		requireErrContains(t, err, "больше нуля")
		if errors.Is(err, ErrUpstream) {
			t.Fatalf("локальная валидация не должна быть ErrUpstream: %v", err)
		}
	}
	if hit {
		t.Fatal("при некорректной сумме запрос к API не должен выполняться")
	}
}

func TestYookassaCreatePaymentUnavailable(t *testing.T) {
	y := newYookassaProvider("shop", "secret")
	y.apiBase = closedServerURL(t)
	_, err := y.CreatePayment(t.Context(), CreatePaymentRequest{
		PaymentID: "idem-1", AmountCents: 4999, Currency: "RUB",
	})
	requireErrIs(t, err, ErrUpstream, "yookassa недоступен")
}

func TestYookassaCreatePaymentTimeout(t *testing.T) {
	y := newYookassaProvider("shop", "secret")
	y.apiBase = hangingServer(t).URL

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := y.CreatePayment(ctx, CreatePaymentRequest{
		PaymentID: "idem-1", AmountCents: 4999, Currency: "RUB",
	})
	requireErrIs(t, err, ErrUpstream, "yookassa недоступен")
}

// ---------- YooKassa: GetStatus ----------

func TestYookassaGetStatusMapping(t *testing.T) {
	cases := []struct {
		providerStatus string
		want           string
	}{
		{"succeeded", "succeeded"},
		{"pending", "pending"},
		{"waiting_for_capture", "pending"},
		{"canceled", "failed"},
		{"unknown_future_status", "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.providerStatus, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				user, pass, ok := r.BasicAuth()
				if ok {
					gotAuth = user + ":" + pass
				}
				fmt.Fprintf(w, `{"id":"yk_42","status":%q}`, tc.providerStatus)
			}))
			defer srv.Close()

			y := newYookassaProvider("shop", "secret")
			y.apiBase = srv.URL
			got, err := y.GetStatus(t.Context(), "yk_42")
			if err != nil {
				t.Fatalf("GetStatus: %v", err)
			}
			if got != tc.want {
				t.Fatalf("GetStatus(%q)=%q, ждали %q", tc.providerStatus, got, tc.want)
			}
			if gotMethod != http.MethodGet || gotPath != "/payments/yk_42" {
				t.Fatalf("запрос %s %s, ждали GET /payments/yk_42", gotMethod, gotPath)
			}
			if gotAuth != "shop:secret" {
				t.Fatalf("Basic-авторизация=%q", gotAuth)
			}
		})
	}
}

func TestYookassaGetStatusAPIErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "404 с description",
			status:  http.StatusNotFound,
			body:    `{"type":"error","description":"Payment not found"}`,
			wantSub: "Payment not found",
		},
		{
			name:    "500 без JSON",
			status:  http.StatusInternalServerError,
			body:    `boom`,
			wantSub: "boom",
		},
		{
			name:    "200 с некорректным JSON",
			status:  http.StatusOK,
			body:    `{"id":`,
			wantSub: "некорректный ответ API",
		},
		{
			name:    "200 без id",
			status:  http.StatusOK,
			body:    `{"status":"succeeded"}`,
			wantSub: "некорректный ответ API",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			y := newYookassaProvider("shop", "secret")
			y.apiBase = srv.URL
			_, err := y.GetStatus(t.Context(), "yk_404")
			requireErrIs(t, err, ErrUpstream, tc.wantSub)
		})
	}
}

// ---------- YooKassa: Refund ----------

func TestYookassaRefundSuccess(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotGetPath, gotPostPath, gotAuth, gotIdem, gotBody, firstIdem string
			refundCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if ok {
					gotAuth = user + ":" + pass
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/payments/yk_1":
					gotGetPath = r.URL.Path
					_, _ = io.WriteString(w, `{"id":"yk_1","status":"succeeded","amount":{"value":"49.99","currency":"RUB"}}`)
				case r.Method == http.MethodPost && r.URL.Path == "/refunds":
					gotPostPath = r.URL.Path
					gotIdem = r.Header.Get("Idempotence-Key")
					refundCalls++
					if refundCalls == 1 {
						firstIdem = gotIdem
					}
					raw, _ := io.ReadAll(r.Body)
					gotBody = string(raw)
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"id":"rf_1","status":"succeeded"}`)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			y := newYookassaProvider("shop", "secret")
			y.apiBase = srv.URL
			if err := y.Refund(t.Context(), "yk_1"); err != nil {
				t.Fatalf("Refund: %v", err)
			}
			// Повторный Refund (например, после таймаута) обязан использовать
			// тот же Idempotence-Key, иначе YooKassa создаст второй возврат.
			if err := y.Refund(t.Context(), "yk_1"); err != nil {
				t.Fatalf("повторный Refund: %v", err)
			}
			if refundCalls != 2 || firstIdem == "" || gotIdem != firstIdem {
				t.Fatalf("Idempotence-Key возврата не детерминирован: calls=%d first=%q last=%q", refundCalls, firstIdem, gotIdem)
			}
			if gotGetPath != "/payments/yk_1" {
				t.Fatalf("не было GET /payments/yk_1 (got %q)", gotGetPath)
			}
			if gotPostPath != "/refunds" {
				t.Fatalf("не было POST /refunds (got %q)", gotPostPath)
			}
			if gotAuth != "shop:secret" {
				t.Fatalf("Basic-авторизация=%q", gotAuth)
			}
			if _, err := uuid.Parse(gotIdem); err != nil {
				t.Fatalf("Idempotence-Key возврата должен быть UUID: %q (%v)", gotIdem, err)
			}
			if !strings.Contains(gotBody, `"payment_id":"yk_1"`) {
				t.Fatalf("в теле возврата нет payment_id: %s", gotBody)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
				t.Fatalf("тело возврата не JSON: %v (%q)", err, gotBody)
			}
			amount, _ := body["amount"].(map[string]any)
			if amount["value"] != "49.99" || amount["currency"] != "RUB" {
				t.Fatalf("amount=%v, ждали 49.99 RUB", body["amount"])
			}
		})
	}
}

func TestYookassaRefundErrors(t *testing.T) {
	cases := []struct {
		name         string
		getStatus    int
		getBody      string
		postStatus   int
		postBody     string
		wantSub      string
		wantUpstream bool
		wantPostHit  bool
	}{
		{
			name:         "платёж не найден (GET 404)",
			getStatus:    http.StatusNotFound,
			getBody:      `{"description":"Payment not found"}`,
			wantSub:      "Payment not found",
			wantUpstream: true,
		},
		{
			name:         "некорректный ответ GET",
			getStatus:    http.StatusOK,
			getBody:      `{"id":`,
			wantSub:      "некорректный ответ API",
			wantUpstream: true,
		},
		{
			name:         "GET без id",
			getStatus:    http.StatusOK,
			getBody:      `{"status":"succeeded","amount":{"value":"1.00","currency":"RUB"}}`,
			wantSub:      "некорректный ответ API",
			wantUpstream: true,
		},
		{
			name:         "GET без суммы — возврат не создаётся",
			getStatus:    http.StatusOK,
			getBody:      `{"id":"yk_1","status":"succeeded"}`,
			wantSub:      "не удалось определить сумму",
			wantUpstream: false,
			wantPostHit:  false,
		},
		{
			name:         "POST 400 с description",
			getStatus:    http.StatusOK,
			getBody:      `{"id":"yk_1","amount":{"value":"49.99","currency":"RUB"}}`,
			postStatus:   http.StatusBadRequest,
			postBody:     `{"description":"Refund amount exceeds payment"}`,
			wantSub:      "Refund amount exceeds payment",
			wantUpstream: true,
			wantPostHit:  true,
		},
		{
			name:         "POST 500 без JSON",
			getStatus:    http.StatusOK,
			getBody:      `{"id":"yk_1","amount":{"value":"49.99","currency":"RUB"}}`,
			postStatus:   http.StatusInternalServerError,
			postBody:     `refund boom`,
			wantSub:      "refund boom",
			wantUpstream: true,
			wantPostHit:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			postHit := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/payments/yk_1" {
					w.WriteHeader(tc.getStatus)
					_, _ = io.WriteString(w, tc.getBody)
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/refunds" {
					postHit = true
					w.WriteHeader(tc.postStatus)
					_, _ = io.WriteString(w, tc.postBody)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer srv.Close()

			y := newYookassaProvider("shop", "secret")
			y.apiBase = srv.URL
			err := y.Refund(t.Context(), "yk_1")
			if tc.wantUpstream {
				requireErrIs(t, err, ErrUpstream, tc.wantSub)
			} else {
				requireErrContains(t, err, tc.wantSub)
				if errors.Is(err, ErrUpstream) {
					t.Fatalf("ожидалась локальная ошибка, а не ErrUpstream: %v", err)
				}
			}
			if postHit != tc.wantPostHit {
				t.Fatalf("POST /refunds hit=%v, ждали %v", postHit, tc.wantPostHit)
			}
		})
	}
}

// ---------- YooKassa: вебхуки (непокрытые ветки) ----------

func TestYookassaHandleWebhookConfirmationBranches(t *testing.T) {
	t.Run("возврат не подтверждён API", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"description":"Refund not found"}`)
		}))
		defer srv.Close()

		y := newYookassaProvider("shop", "secret")
		y.apiBase = srv.URL
		body := `{"type":"refund.succeeded","object":{"id":"rf_fake","payment_id":"yk_1"}}`
		_, err := y.HandleWebhook(httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body)))
		if err == nil {
			t.Fatal("неподтверждённый возврат прошёл обработку")
		}
		if !errors.Is(err, ErrUpstream) {
			t.Fatalf("ошибка не оборачивает ErrUpstream: %v", err)
		}
	})

	t.Run("возврат без payment_id — id возврата", func(t *testing.T) {
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_, _ = io.WriteString(w, `{"id":"rf_1","payment_id":"yk_confirmed","status":"succeeded"}`)
		}))
		defer srv.Close()

		y := newYookassaProvider("shop", "secret")
		y.apiBase = srv.URL
		body := `{"type":"refund.succeeded","object":{"id":"rf_1"}}`
		ev, err := y.HandleWebhook(httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body)))
		if err != nil {
			t.Fatalf("HandleWebhook: %v", err)
		}
		if gotPath != "/refunds/rf_1" {
			t.Fatalf("подтверждение возврата по пути %q", gotPath)
		}
		if ev.ProviderPaymentID != "yk_confirmed" || ev.Status != "refunded" {
			t.Fatalf("событие %+v, ждали id=yk_confirmed status=refunded", ev)
		}
	})

	t.Run("платёж не подтверждён корректным JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"id":`)
		}))
		defer srv.Close()

		y := newYookassaProvider("shop", "secret")
		y.apiBase = srv.URL
		body := `{"type":"payment.succeeded","object":{"id":"yk_1","status":"succeeded"}}`
		_, err := y.HandleWebhook(httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body)))
		if err == nil {
			t.Fatal("платёж с некорректным ответом API прошёл обработку")
		}
	})
}

func TestYookassaVerifyWebhookUntrustedSource(t *testing.T) {
	y := newYookassaProvider("shop", "secret")
	y.trusted = parseTrustedNets("10.0.0.0/8")
	body := `{"type":"payment.succeeded","object":{"id":"yk_1","status":"succeeded"}}`

	untrusted := httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body))
	untrusted.RemoteAddr = "8.8.8.8:1234"
	err := y.VerifyWebhook(untrusted, nil)
	requireErrIs(t, err, ErrUnauthorized, "недоверенного IP")

	trusted := httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body))
	trusted.RemoteAddr = "10.1.2.3:1234"
	if err := y.VerifyWebhook(trusted, nil); err != nil {
		t.Fatalf("доверенный источник отклонён: %v", err)
	}
}

// TestYookassaInvalidTrustedIPsDenyAll — опечатка в YOOKASSA_TRUSTED_IPS не
// должна молча отключать проверку источника: конфигурация «задана, но пуста»
// отклоняет все вебхуки.
func TestYookassaInvalidTrustedIPsDenyAll(t *testing.T) {
	y := newYookassaProvider("shop", "secret")
	y.trustedConfigured = true
	y.trusted = parseTrustedNets("не-ip, 999.999.999.999/33")
	body := `{"type":"payment.succeeded","object":{"id":"yk_1","status":"succeeded"}}`
	r := httptest.NewRequest(http.MethodPost, "/payments/webhook/yookassa", strings.NewReader(body))
	r.RemoteAddr = "10.1.2.3:1234"
	requireErrIs(t, y.VerifyWebhook(r, nil), ErrUnauthorized, "YOOKASSA_TRUSTED_IPS")
}

// ---------- Mock: непокрытые ветки ----------

func TestMockVerifyWebhookSignaturePrefix(t *testing.T) {
	m := NewMockProvider()
	secret := []byte("mock-secret")
	body := []byte(`{"providerPaymentId":"mock_1","status":"succeeded"}`)
	sig := mockHMACHex(secret, body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock", strings.NewReader(string(body)))
	r.Header.Set("X-Mock-Signature", "sha256="+sig)
	if err := m.VerifyWebhook(r, secret); err != nil {
		t.Fatalf("подпись с префиксом sha256= отклонена: %v", err)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock", strings.NewReader(string(body)))
	r.Header.Set("X-Mock-Signature", "sha256=not-hex")
	requireErrIs(t, m.VerifyWebhook(r, secret), ErrUnauthorized, "hex")
}

func TestMockHandleWebhookAndRefund(t *testing.T) {
	m := NewMockProvider()

	// Некорректное тело вебхука — ошибка без паники.
	if _, err := m.HandleWebhook(httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock",
		strings.NewReader(`не json`))); err == nil {
		t.Fatal("некорректный вебхук принят")
	}
	if _, err := m.HandleWebhook(httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook/mock",
		strings.NewReader(`{"status":"succeeded"}`))); err == nil {
		t.Fatal("вебхук без providerPaymentId принят")
	}

	res, err := m.CreatePayment(t.Context(), CreatePaymentRequest{AmountCents: 4900, Currency: "RUB"})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if err := m.Refund(t.Context(), res.ProviderPaymentID); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if st, err := m.GetStatus(t.Context(), res.ProviderPaymentID); err != nil || st != "refunded" {
		t.Fatalf("GetStatus=%q err=%v, ждали refunded", st, err)
	}
	if err := m.Refund(t.Context(), "mock_unknown"); err == nil {
		t.Fatal("возврат неизвестного платежа не вернул ошибку")
	}
}
