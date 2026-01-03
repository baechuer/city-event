package memory

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
)

type OAuthStateStore struct {
	mu     sync.Mutex
	states map[string]stateEntry
}

type stateEntry struct {
	data      auth.OAuthStateData
	expiresAt time.Time
}

func NewOAuthStateStore() *OAuthStateStore {
	return &OAuthStateStore{
		states: make(map[string]stateEntry),
	}
}

func (s *OAuthStateStore) Create(ctx context.Context, state auth.OAuthStateData) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cleanup expired
	now := time.Now()
	for k, v := range s.states {
		if now.After(v.expiresAt) {
			delete(s.states, k)
		}
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(stateBytes)

	s.states[token] = stateEntry{
		data:      state,
		expiresAt: time.Now().Add(10 * time.Minute), // Hardcoded TTL for memory store
	}

	return token, nil
}

func (s *OAuthStateStore) Consume(ctx context.Context, token string) (auth.OAuthStateData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.states[token]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.states, token)
		return auth.OAuthStateData{}, errors.New("oauth state not found or expired")
	}

	delete(s.states, token) // One-time use
	return entry.data, nil
}
