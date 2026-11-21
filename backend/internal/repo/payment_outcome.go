package repo

// ApplyPaymentOutcome commits payment status, promo usage and access together.
// The caller holds AcquireBillingLock, shared by all API replicas.
func (s *PostgresStore) ApplyPaymentOutcome(id, event string) (string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	p, err := scanPayment(tx.QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id=$1::uuid FOR UPDATE`, id).Scan)
	if err != nil {
		return "", err
	}
	result := "ignored"
	switch event {
	case "succeeded":
		if p.Status == PaymentRefunded {
			return result, nil
		}
		result = "duplicate"
		if p.Status != PaymentSucceeded {
			result = "processed"
			if _, err = tx.Exec(ctx, `UPDATE payments SET status='succeeded', processed_at=now() WHERE id=$1::uuid`, id); err != nil {
				return "", err
			}
			if p.PromoCode != "" {
				if _, err = tx.Exec(ctx, `UPDATE promo_codes SET used_count=used_count+1 WHERE course_id=$1::uuid AND code=$2`, p.CourseID, p.PromoCode); err != nil {
					return "", err
				}
			}
		}
		// Replays never undo a manual revoke. A genuinely new paid attempt
		// can reactivate access, preserving the established repurchase policy.
		if result == "processed" {
			_, err = tx.Exec(ctx, `INSERT INTO enrollments(student_id,course_id,status) VALUES($1::uuid,$2::uuid,'active')
			 ON CONFLICT(student_id,course_id) DO UPDATE SET status='active'`, p.StudentID, p.CourseID)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO enrollments(student_id,course_id,status) VALUES($1::uuid,$2::uuid,'active') ON CONFLICT(student_id,course_id) DO NOTHING`, p.StudentID, p.CourseID)
		}
	case "failed":
		if p.Status == PaymentFailed {
			result = "duplicate"
		}
		if p.Status == PaymentPending {
			result = "processed"
			_, err = tx.Exec(ctx, `UPDATE payments SET status='failed',processed_at=now() WHERE id=$1::uuid`, id)
		}
	case "refunded":
		if p.Status != PaymentSucceeded && p.Status != PaymentRefunded {
			return result, nil
		}
		result = "duplicate"
		if p.Status == PaymentSucceeded {
			result = "processed"
			if _, err = tx.Exec(ctx, `UPDATE payments SET status='refunded',processed_at=now() WHERE id=$1::uuid`, id); err != nil {
				return "", err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE enrollments SET status='refunded' WHERE student_id=$1::uuid AND course_id=$2::uuid AND status!='revoked'
		 AND NOT EXISTS(SELECT 1 FROM payments WHERE student_id=$1::uuid AND course_id=$2::uuid AND status='succeeded')`, p.StudentID, p.CourseID)
	default:
		return result, nil
	}
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return result, nil
}
