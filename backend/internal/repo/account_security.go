package repo

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
)

const PasswordReset = "password_reset"
const EmailVerification = "email_verification"
const AccountTokenCooldown = time.Minute

var ErrTokenRateLimited = errors.New("запросить новое письмо можно через минуту")
var ErrInvalidAccountToken = errors.New("ссылка недействительна, уже использована или истекла")

type AccountToken struct {
	UserID, Purpose, Hash string
	CreatedAt, ExpiresAt  time.Time
}

func (s *Store) IssueAccountToken(userID, expectedHash, purpose, hash string, now, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok || u.Status == UserBlocked || u.PasswordHash != expectedHash {
		return ErrConflict
	}
	if purpose != PasswordReset && purpose != EmailVerification {
		return ErrConflict
	}
	if !expires.After(now) {
		return ErrConflict
	}
	key := userID + "|" + purpose
	if previous, ok := s.accountTokens[key]; ok && now.Sub(previous.CreatedAt) < AccountTokenCooldown {
		return ErrTokenRateLimited
	}
	// Expired tokens are never useful and must not accumulate in the dev store.
	for key, token := range s.accountTokens {
		if !token.ExpiresAt.After(now) {
			delete(s.accountTokens, key)
		}
	}
	s.accountTokens[key] = &AccountToken{UserID: userID, Purpose: purpose, Hash: hash, CreatedAt: now, ExpiresAt: expires}
	return nil
}

func (s *Store) DiscardAccountToken(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, token := range s.accountTokens {
		if token.Hash == hash {
			delete(s.accountTokens, key)
		}
	}
	return nil
}

func (s *Store) ConsumeAccountToken(hash, purpose, passwordHash string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var token *AccountToken
	key := ""
	for candidate, value := range s.accountTokens {
		if value.Hash == hash && value.Purpose == purpose {
			token = value
			key = candidate
			break
		}
	}
	if token == nil || !token.ExpiresAt.After(now) {
		return ErrInvalidAccountToken
	}
	u, ok := s.users[token.UserID]
	if !ok || u.Status == UserBlocked {
		return ErrInvalidAccountToken
	}
	switch purpose {
	case PasswordReset:
		if passwordHash == "" {
			return ErrConflict
		}
		u.PasswordHash = passwordHash
		for id, session := range s.sessions {
			if session.UserID == u.ID {
				delete(s.sessions, id)
			}
		}
		for key, value := range s.accountTokens {
			if value.UserID == u.ID {
				delete(s.accountTokens, key)
			}
		}
	case EmailVerification:
		if u.EmailVerifiedAt == nil {
			verified := now
			u.EmailVerifiedAt = &verified
		}
		delete(s.accountTokens, key)
	default:
		return ErrInvalidAccountToken
	}
	return nil
}

// ClaimEmailOwnership — см. StoreAPI: email подтверждён внешним провайдером.
func (s *Store) ClaimEmailOwnership(userID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok || u.Status == UserBlocked {
		return ErrNotFound
	}
	if u.EmailVerifiedAt == nil {
		now := time.Now().UTC()
		u.EmailVerifiedAt = &now
	}
	u.PasswordHash = passwordHash
	for id, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, id)
		}
	}
	for key, token := range s.accountTokens {
		if token.UserID == userID {
			delete(s.accountTokens, key)
		}
	}
	return nil
}

func (s *Store) CreateAccountSession(userID, expectedHash string, ttl time.Duration, userAgent, ip string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok || u.Status == UserBlocked || u.PasswordHash != expectedHash {
		return nil, ErrConflict
	}
	now := time.Now().UTC()
	session := &Session{ID: uuid.NewString(), PublicID: uuid.NewString(), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl), LastSeenAt: now, UserAgent: userAgent, IPAddress: ip}
	s.sessions[session.ID] = session
	copy := *session
	return &copy, nil
}

func (s *Store) ListAccountSessions(userID string) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	result := make([]*Session, 0)
	for _, session := range s.sessions {
		if session.UserID == userID && session.ExpiresAt.After(now) {
			copy := *session
			result = append(result, &copy)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) RevokeAccountSession(userID, publicID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if session.UserID == userID && session.PublicID == publicID {
			delete(s.sessions, id)
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) RevokeOtherAccountSessions(userID, keepID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if session.UserID == userID && id != keepID {
			delete(s.sessions, id)
		}
	}
	return nil
}
