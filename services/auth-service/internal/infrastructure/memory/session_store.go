package memory

import (
	"context"
	"sync"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// tokenEntry holds the user ID and expiration time for a refresh token
type tokenEntry struct {
	userID    string
	expiresAt time.Time
}

type SessionStore struct {
	mu sync.RWMutex
	// refreshToken -> tokenEntry (userID + expiresAt)
	tokenToEntry map[string]tokenEntry
	// userID -> set(refreshToken)
	userTokens map[string]map[string]struct{}
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		tokenToEntry: make(map[string]tokenEntry),
		userTokens:   make(map[string]map[string]struct{}),
	}
}

func (s *SessionStore) CreateRefreshToken(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	tok, err := newOpaqueToken(32)
	if err != nil {
		return "", domain.ErrRandomFailed(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Store with expiration time
	s.tokenToEntry[tok] = tokenEntry{
		userID:    userID,
		expiresAt: time.Now().Add(ttl),
	}
	if s.userTokens[userID] == nil {
		s.userTokens[userID] = make(map[string]struct{})
	}
	s.userTokens[userID][tok] = struct{}{}

	return tok, nil
}

func (s *SessionStore) GetUserIDByRefreshToken(ctx context.Context, token string) (string, error) {
	s.mu.RLock()
	entry, ok := s.tokenToEntry[token]
	s.mu.RUnlock()

	if !ok {
		return "", domain.ErrRefreshTokenInvalid()
	}

	// Validate expiration
	if time.Now().After(entry.expiresAt) {
		// Token expired - revoke it and return error
		_ = s.RevokeRefreshToken(ctx, token)
		return "", domain.ErrRefreshTokenInvalid()
	}

	return entry.userID, nil
}

func (s *SessionStore) RotateRefreshToken(ctx context.Context, oldToken string, ttl time.Duration) (string, error) {
	uid, err := s.GetUserIDByRefreshToken(ctx, oldToken)
	if err != nil {
		return "", err
	}

	// revoke old
	_ = s.RevokeRefreshToken(ctx, oldToken)

	// create new
	return s.CreateRefreshToken(ctx, uid, ttl)
}

func (s *SessionStore) RevokeRefreshToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.tokenToEntry[token]
	if !ok {
		return nil // idempotent
	}
	delete(s.tokenToEntry, token)
	if set := s.userTokens[entry.userID]; set != nil {
		delete(set, token)
		if len(set) == 0 {
			delete(s.userTokens, entry.userID)
		}
	}
	return nil
}

func (s *SessionStore) RevokeAll(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	set := s.userTokens[userID]
	for tok := range set {
		delete(s.tokenToEntry, tok)
	}
	delete(s.userTokens, userID)
	return nil
}

// local helper (same as in service.go but duplicated to avoid package import cycle)
func newOpaqueToken(bytesLen int) (string, error) {
	return domainInternalOpaqueToken(bytesLen)
}
