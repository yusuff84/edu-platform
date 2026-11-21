package repo

// SetPaymentCheckout attaches the provider result to the durable purchase attempt.
func (s *Store) SetPaymentCheckout(id, providerID, redirectURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return ErrNotFound
	}
	if existing, ok := s.paymentsByProvider[providerID]; ok && existing.ID != id {
		return ErrAlreadyExists
	}
	if p.ProviderPaymentID != "" && p.ProviderPaymentID != providerID {
		return ErrConflict
	}
	p.ProviderPaymentID, p.RedirectURL = providerID, redirectURL
	s.paymentsByProvider[providerID] = p
	return nil
}

func (s *PostgresStore) SetPaymentCheckout(id, providerID, redirectURL string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE payments SET provider_payment_id=$2, redirect_url=$3
		WHERE id=$1::uuid AND (provider_payment_id IS NULL OR provider_payment_id=$2)`, id, providerID, redirectURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
