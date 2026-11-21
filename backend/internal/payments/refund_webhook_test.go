package payments

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYookassaRefundUsesVerifiedPaymentAndStatus(t *testing.T) {
	for _, status := range []string{"succeeded", "pending", "canceled"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"id":"rf_verified","payment_id":"yk_actual","status":%q}`, status)
			}))
			defer srv.Close()
			provider := newYookassaProvider("shop", "secret")
			provider.apiBase = srv.URL
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"type":"refund.succeeded","object":{"id":"rf_verified","payment_id":"yk_victim"}}`))
			event, err := provider.HandleWebhook(request)
			if err != nil {
				t.Fatal(err)
			}
			if event.ProviderPaymentID != "yk_actual" {
				t.Fatalf("trusted caller-supplied payment id: %+v", event)
			}
			if (event.Status == "refunded") != (status == "succeeded") {
				t.Fatalf("unconfirmed refund: %+v", event)
			}
		})
	}
}
