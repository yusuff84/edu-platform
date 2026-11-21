package payments

import (
	"errors"
	"testing"
)

func TestDisabledProviderCannotCreatePayments(t *testing.T) {
	provider, err := NewProvider("disabled")
	if provider != nil || !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("disabled provider must be unavailable: %v %v", provider, err)
	}
}
