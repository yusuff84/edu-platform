package repo

// Postgres-реализация журнала действий администраторов (in-memory — admin_audit.go).

import (
	"log"
	"strings"
)

func (s *PostgresStore) AddAuditEntry(actorID, actorEmail, action, ip string) {
	ctx, cancel := s.ctx()
	defer cancel()
	var actor any
	if validUUID(actorID) {
		actor = actorID
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO admin_audit (actor_id, actor_email, action, ip) VALUES ($1::uuid,$2,$3,$4)`,
		actor, actorEmail, action, ip); err != nil {
		log.Printf("postgres AddAuditEntry: %v", err)
	}
}

func (s *PostgresStore) ListAuditEntries(q string, page, limit int) ([]*AuditEntry, int) {
	out := make([]*AuditEntry, 0)
	ctx, cancel := s.ctx()
	defer cancel()
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	const where = ` WHERE ($1 = '%%' OR lower(action) LIKE $1 OR lower(actor_email) LIKE $1)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM admin_audit`+where, like).Scan(&total); err != nil {
		log.Printf("postgres ListAuditEntries count: %v", err)
		return out, 0
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, coalesce(actor_id::text,''), actor_email, action, ip, created_at FROM admin_audit`+where+
			` ORDER BY created_at DESC LIMIT $2 OFFSET $3`, like, limit, (page-1)*limit)
	if err != nil {
		log.Printf("postgres ListAuditEntries: %v", err)
		return out, total
	}
	defer rows.Close()
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorEmail, &e.Action, &e.IP, &e.CreatedAt); err != nil {
			log.Printf("postgres ListAuditEntries scan: %v", err)
			return out, total
		}
		out = append(out, &e)
	}
	return out, total
}
