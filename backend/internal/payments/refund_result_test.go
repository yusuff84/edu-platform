package payments

import "testing"

func TestRefundRequiresConfirmedSuccess(t *testing.T) {
	for _, body := range []string{`{"id":"refund","status":"pending"}`, `{"id":"refund","status":"failed"}`, `{"id":"refund"}`, `{}`, `broken`} {
		if err := confirmedRefund([]byte(body)); err == nil {
			t.Fatalf("unconfirmed refund accepted: %s", body)
		}
	}
	if err := confirmedRefund([]byte(`{"id":"refund","status":"succeeded"}`)); err != nil {
		t.Fatal(err)
	}
}
