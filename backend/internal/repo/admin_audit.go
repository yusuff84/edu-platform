package repo

// Журнал действий администраторов (in-memory). Postgres — postgres_admin_audit.go,
// схема — 020_admin_audit.sql.

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// AuditEntry — одно действие администратора.
type AuditEntry struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actorId"`
	ActorEmail string    `json:"actorEmail"`
	Action     string    `json:"action"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
}

// auditMemLimit — сколько записей держит in-memory журнал (старые вытесняются).
const auditMemLimit = 5000

func (s *Store) AddAuditEntry(actorID, actorEmail, action, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, &AuditEntry{ID: uuid.NewString(), ActorID: actorID, ActorEmail: actorEmail,
		Action: action, IP: ip, CreatedAt: time.Now().UTC()})
	if len(s.audit) > auditMemLimit {
		s.audit = s.audit[len(s.audit)-auditMemLimit:]
	}
}

// ListAuditEntries — записи журнала, новые первыми; q — подстрока по действию или email.
func (s *Store) ListAuditEntries(q string, page, limit int) ([]*AuditEntry, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q = strings.ToLower(strings.TrimSpace(q))
	matched := make([]*AuditEntry, 0)
	for i := len(s.audit) - 1; i >= 0; i-- {
		e := s.audit[i]
		if q != "" && !strings.Contains(strings.ToLower(e.Action), q) && !strings.Contains(strings.ToLower(e.ActorEmail), q) {
			continue
		}
		c := *e
		matched = append(matched, &c)
	}
	total := len(matched)
	start := (page - 1) * limit
	if start >= total {
		return []*AuditEntry{}, total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return matched[start:end], total
}
