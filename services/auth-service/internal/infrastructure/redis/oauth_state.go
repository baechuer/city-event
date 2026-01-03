package redis

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	goredis "github.com/redis/go-redis/v9"
)

// OAuthStateStore manages OAuth state tokens in Redis
type OAuthStateStore struct {
	client *Client
	ttl    time.Duration
}

// NewOAuthStateStore creates a new OAuth state store
func NewOAuthStateStore(client *Client, ttl time.Duration) *OAuthStateStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &OAuthStateStore{
		client: client,
		ttl:    ttl,
	}
}

// Create generates a new state token and stores the state in Redis
// Returns the state token to use in the OAuth flow
func (s *OAuthStateStore) Create(ctx context.Context, state auth.OAuthStateData) (string, error) {
	// Generate random state token
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	stateToken := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Serialize state
	data, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("failed to marshal state: %w", err)
	}

	// Store in Redis with TTL using underlying client
	key := "oauth:state:" + stateToken
	if err := s.client.rdb.Set(ctx, key, data, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to store state: %w", err)
	}

	return stateToken, nil
}

// Consume retrieves and deletes the state from Redis (one-time use)
// This prevents replay attacks
func (s *OAuthStateStore) Consume(ctx context.Context, stateToken string) (auth.OAuthStateData, error) {
	key := "oauth:state:" + stateToken

	// Get and delete atomically using a transaction
	var state auth.OAuthStateData
	err := s.client.rdb.Watch(ctx, func(tx *goredis.Tx) error {
		data, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, goredis.Nil) {
				return ErrStateNotFound
			}
			return err
		}

		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("failed to unmarshal state: %w", err)
		}

		// Delete the key to prevent replay
		_, err = tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.Del(ctx, key)
			return nil
		})
		return err
	}, key)

	if err != nil {
		return auth.OAuthStateData{}, err
	}

	return state, nil
}

// ErrStateNotFound is returned when the state token is not found or expired
var ErrStateNotFound = errors.New("oauth state not found or expired")
