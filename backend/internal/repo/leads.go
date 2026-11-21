package repo

// Заявки на консультацию с главной страницы. In-memory реализация;
// Postgres-зеркало — postgres_leads.go, схема — 018_leads.sql.

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// Lead — заявка «Поможем подобрать курс».
type Lead struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Phone         string    `json:"phone"`
	Email         string    `json:"email"`
	ContactMethod string    `json:"contactMethod"`
	Status        string    `json:"status"`
	IPAddress     string    `json:"-"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Статусы заявки: новая и обработанная менеджером.
const (
	LeadStatusNew  = "new"
	LeadStatusDone = "done"
)

// CreateLead сохраняет заявку (поля уже провалидированы обработчиком).
func (s *Store) CreateLead(l Lead) (*Lead, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l.ID = uuid.NewString()
	l.Status = LeadStatusNew
	l.CreatedAt = time.Now().UTC()
	stored := l
	s.leads = append(s.leads, &stored)
	out := stored
	return &out, nil
}

// ListLeads — заявки, новые первыми, не больше limit. Никогда не nil.
func (s *Store) ListLeads(limit int) []*Lead {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Lead, 0, len(s.leads))
	for _, l := range s.leads {
		c := *l
		out = append(out, &c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SetLeadStatus меняет статус заявки; ErrNotFound — заявки нет.
func (s *Store) SetLeadStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.leads {
		if l.ID == id {
			l.Status = status
			return nil
		}
	}
	return ErrNotFound
}
