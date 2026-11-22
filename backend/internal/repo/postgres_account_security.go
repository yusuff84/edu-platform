package repo

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) IssueAccountToken(userID, expectedHash, purpose, hash string, now, expires time.Time) error {
	if (purpose != PasswordReset && purpose != EmailVerification) || !expires.After(now) {
		return ErrConflict
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid AND password_hash=$2 AND status!='blocked' FOR UPDATE`, userID, expectedHash).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO account_tokens(user_id,purpose,token_hash,created_at,expires_at) VALUES($1::uuid,$2,$3,$4,$5)
	 ON CONFLICT(user_id,purpose) DO UPDATE SET token_hash=EXCLUDED.token_hash,created_at=EXCLUDED.created_at,expires_at=EXCLUDED.expires_at
	 WHERE account_tokens.created_at <= $6`, userID, purpose, hash, now, expires, now.Add(-AccountTokenCooldown))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrTokenRateLimited
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) DiscardAccountToken(hash string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM account_tokens WHERE token_hash=$1`, hash)
	return err
}

func (s *PostgresStore) ConsumeAccountToken(hash, purpose, passwordHash string, now time.Time) error {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM account_tokens WHERE token_hash=$1 AND purpose=$2`, hash, purpose).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidAccountToken
	}
	if err != nil {
		return err
	}
	// Lock the user first in issue, consume and session creation. A password
	// reset and an in-flight login cannot leave an old-password session alive.
	err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid AND status!='blocked' FOR UPDATE`, userID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidAccountToken
	}
	if err != nil {
		return err
	}
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT expires_at FROM account_tokens WHERE user_id=$1::uuid AND purpose=$2 AND token_hash=$3 FOR UPDATE`, userID, purpose, hash).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidAccountToken
	}
	if err != nil {
		return err
	}
	if !expires.After(now) {
		return ErrInvalidAccountToken
	}
	switch purpose {
	case PasswordReset:
		if passwordHash == "" {
			return ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1::uuid`, userID, passwordHash); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1::uuid`, userID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM account_tokens WHERE user_id=$1::uuid`, userID)
	case EmailVerification:
		if _, err = tx.Exec(ctx, `UPDATE users SET email_verified_at=COALESCE(email_verified_at,$2) WHERE id=$1::uuid`, userID, now); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM account_tokens WHERE user_id=$1::uuid AND purpose=$2`, userID, purpose)
	default:
		return ErrInvalidAccountToken
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ClaimEmailOwnership — см. StoreAPI: email подтверждён внешним провайдером.
func (s *PostgresStore) ClaimEmailOwnership(userID, passwordHash string) error {
	if !validUUID(userID) {
		return ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx,
		`UPDATE users SET password_hash=$2, email_verified_at=COALESCE(email_verified_at, now())
		 WHERE id=$1::uuid AND status!='blocked'`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1::uuid`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM account_tokens WHERE user_id=$1::uuid`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CreateAccountSession(userID, expectedHash string, ttl time.Duration, userAgent, ip string) (*Session, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1::uuid AND password_hash=$2 AND status!='blocked' FOR SHARE`, userID, expectedHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	session := &Session{ID: uuid.NewString(), PublicID: uuid.NewString(), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl), LastSeenAt: now, UserAgent: userAgent, IPAddress: ip}
	_, err = tx.Exec(ctx, `INSERT INTO sessions(id,public_id,user_id,created_at,expires_at,last_seen_at,user_agent,ip_address) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$4,$6,$7)`, session.ID, session.PublicID, userID, now, session.ExpiresAt, userAgent, ip)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *PostgresStore) ListAccountSessions(userID string) ([]*Session, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT id::text,public_id::text,user_id::text,created_at,expires_at,last_seen_at,user_agent,ip_address FROM sessions WHERE user_id=$1::uuid AND expires_at>now() ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*Session, 0)
	for rows.Next() {
		session := &Session{}
		if err = rows.Scan(&session.ID, &session.PublicID, &session.UserID, &session.CreatedAt, &session.ExpiresAt, &session.LastSeenAt, &session.UserAgent, &session.IPAddress); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, rows.Err()
}

func (s *PostgresStore) RevokeAccountSession(userID, publicID string) (bool, error) {
	if _, err := uuid.Parse(publicID); err != nil {
		return false, nil
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1::uuid AND public_id=$2::uuid`, userID, publicID)
	return tag.RowsAffected() > 0, err
}

func (s *PostgresStore) RevokeOtherAccountSessions(userID, keepID string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1::uuid AND id!=$2::uuid`, userID, keepID)
	return err
}
