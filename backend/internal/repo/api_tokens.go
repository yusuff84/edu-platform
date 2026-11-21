package repo

// Токены MCP-доступа (in-memory). Postgres — postgres_api_tokens.go, схема — 019_api_tokens.sql.

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// APIToken — токен доступа к MCP-серверу. Сам токен не хранится — только хеш.
type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	UserEmail  string     `json:"userEmail"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

func (s *Store) CreateAPIToken(userID, name, hash, prefix string) (*APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return nil, ErrNotFound
	}
	t := &APIToken{ID: uuid.NewString(), UserID: userID, Name: name, TokenHash: hash, Prefix: prefix, CreatedAt: time.Now().UTC()}
	s.apiTokens = append(s.apiTokens, t)
	c := *t
	return &c, nil
}

func (s *Store) GetAPITokenByHash(hash string) (*APIToken, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.apiTokens {
		if t.TokenHash == hash {
			c := *t
			return &c, true
		}
	}
	return nil, false
}

func (s *Store) ListAPITokens() []*APIToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*APIToken, 0, len(s.apiTokens))
	for _, t := range s.apiTokens {
		c := *t
		if u, ok := s.users[t.UserID]; ok {
			c.UserEmail = u.Email
		}
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) RevokeAPIToken(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.apiTokens {
		if t.ID == id && t.RevokedAt == nil {
			now := time.Now().UTC()
			t.RevokedAt = &now
			return true
		}
	}
	return false
}

func (s *Store) TouchAPIToken(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.apiTokens {
		if t.ID == id {
			now := time.Now().UTC()
			t.LastUsedAt = &now
			return
		}
	}
}
