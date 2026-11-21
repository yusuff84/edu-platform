package payments

import (
	"encoding/json"
	"fmt"
)

// HTTP acceptance is not proof of completed refund. Until the provider confirms
// success, retain local paid status and allow retry with the same idempotency key.
func confirmedRefund(raw []byte) error {
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.ID == "" {
		return fmt.Errorf("%w: некорректный ответ API возврата", ErrUpstream)
	}
	if result.Status != "succeeded" {
		return fmt.Errorf("%w: возврат ещё не подтверждён провайдером", ErrUpstream)
	}
	return nil
}
