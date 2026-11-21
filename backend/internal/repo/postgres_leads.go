package repo

// Postgres-реализация заявок на консультацию (in-memory — leads.go).

import (
	"fmt"
	"log"
)

// CreateLead сохраняет заявку.
func (s *PostgresStore) CreateLead(l Lead) (*Lead, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	out := l
	err := s.pool.QueryRow(ctx,
		`INSERT INTO leads (name, phone, email, contact_method, ip_address)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id::text, status, created_at`,
		l.Name, l.Phone, l.Email, l.ContactMethod, l.IPAddress,
	).Scan(&out.ID, &out.Status, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgres CreateLead: %w", err)
	}
	return &out, nil
}

// ListLeads — заявки, новые первыми, не больше limit. Никогда не nil.
func (s *PostgresStore) ListLeads(limit int) []*Lead {
	out := make([]*Lead, 0)
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, name, phone, email, contact_method, status, ip_address, created_at
		   FROM leads ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		log.Printf("postgres ListLeads: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var l Lead
		if err := rows.Scan(&l.ID, &l.Name, &l.Phone, &l.Email, &l.ContactMethod, &l.Status, &l.IPAddress, &l.CreatedAt); err != nil {
			log.Printf("postgres ListLeads scan: %v", err)
			continue
		}
		out = append(out, &l)
	}
	return out
}

// SetLeadStatus меняет статус заявки; ErrNotFound — заявки нет или id не UUID.
func (s *PostgresStore) SetLeadStatus(id, status string) error {
	if !validUUID(id) {
		return ErrNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE leads SET status=$2 WHERE id=$1::uuid`, id, status)
	if err != nil {
		if isInvalidUUID(err) {
			return ErrNotFound
		}
		return fmt.Errorf("postgres SetLeadStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
