package repo

// Postgres-реализация токенов MCP-доступа (in-memory — api_tokens.go).

import (
	"fmt"
	"log"
)

const apiTokenCols = `t.id::text, t.user_id::text, coalesce(u.email,''), t.name, t.token_hash, t.prefix, t.created_at, t.last_used_at, t.revoked_at`

func scanAPIToken(scan func(...any) error) (*APIToken, error) {
	var t APIToken
	if err := scan(&t.ID, &t.UserID, &t.UserEmail, &t.Name, &t.TokenHash, &t.Prefix, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *PostgresStore) CreateAPIToken(userID, name, hash, prefix string) (*APIToken, error) {
	if !validUUID(userID) {
		return nil, ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	var id string
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, prefix) VALUES ($1::uuid,$2,$3,$4) RETURNING id::text`,
		userID, name, hash, prefix).Scan(&id); err != nil {
		return nil, fmt.Errorf("postgres CreateAPIToken: %w", err)
	}
	row := s.pool.QueryRow(ctx, `SELECT `+apiTokenCols+` FROM api_tokens t LEFT JOIN users u ON u.id=t.user_id WHERE t.id=$1::uuid`, id)
	return scanAPIToken(row.Scan)
}

func (s *PostgresStore) GetAPITokenByHash(hash string) (*APIToken, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	row := s.pool.QueryRow(ctx, `SELECT `+apiTokenCols+` FROM api_tokens t LEFT JOIN users u ON u.id=t.user_id WHERE t.token_hash=$1`, hash)
	t, err := scanAPIToken(row.Scan)
	if err != nil {
		return nil, false
	}
	return t, true
}

func (s *PostgresStore) ListAPITokens() []*APIToken {
	out := make([]*APIToken, 0)
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT `+apiTokenCols+` FROM api_tokens t LEFT JOIN users u ON u.id=t.user_id ORDER BY t.created_at DESC LIMIT 200`)
	if err != nil {
		log.Printf("postgres ListAPITokens: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if t, err := scanAPIToken(rows.Scan); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func (s *PostgresStore) RevokeAPIToken(id string) bool {
	if !validUUID(id) {
		return false
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE api_tokens SET revoked_at=now() WHERE id=$1::uuid AND revoked_at IS NULL`, id)
	return err == nil && tag.RowsAffected() > 0
}

func (s *PostgresStore) TouchAPIToken(id string) {
	ctx, cancel := s.ctx()
	defer cancel()
	if _, err := s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at=now() WHERE id=$1::uuid`, id); err != nil {
		log.Printf("postgres TouchAPIToken: %v", err)
	}
}
