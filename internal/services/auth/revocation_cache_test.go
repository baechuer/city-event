package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCachedRevocationRepositoryWritesThroughAndFallsBack(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	cache := newMemoryRevocationCache()
	cached := WithRevocationCache(repo, cache)

	expiresAt := time.Now().Add(time.Hour)
	if err := cached.RevokeToken(ctx, "token-1", "user-1", expiresAt); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if revoked, err := cache.IsRevoked(ctx, "token-1"); err != nil || !revoked {
		t.Fatalf("expected token to be cached as revoked, revoked=%v err=%v", revoked, err)
	}

	if revoked, err := cached.IsTokenRevoked(ctx, "token-1"); err != nil || !revoked {
		t.Fatalf("expected cached token to be revoked, revoked=%v err=%v", revoked, err)
	}

	if err := repo.RevokeToken(ctx, "token-2", "user-1", expiresAt); err != nil {
		t.Fatalf("direct revoke token: %v", err)
	}
	if revoked, err := cached.IsTokenRevoked(ctx, "token-2"); err != nil || !revoked {
		t.Fatalf("expected repository fallback to find revoked token, revoked=%v err=%v", revoked, err)
	}
	if revoked, err := cache.IsRevoked(ctx, "token-2"); err != nil || !revoked {
		t.Fatalf("expected fallback result to warm cache, revoked=%v err=%v", revoked, err)
	}
}

func TestCachedRevocationRepositoryIgnoresCacheOutage(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	cache := newMemoryRevocationCache()
	cache.err = errors.New("redis unavailable")
	cached := WithRevocationCache(repo, cache)

	expiresAt := time.Now().Add(time.Hour)
	if err := cached.RevokeToken(ctx, "token-1", "user-1", expiresAt); err != nil {
		t.Fatalf("revoke token should not fail on cache outage: %v", err)
	}
	if revoked, err := cached.IsTokenRevoked(ctx, "token-1"); err != nil || !revoked {
		t.Fatalf("expected repository fallback on cache outage, revoked=%v err=%v", revoked, err)
	}
}

type memoryRevocationCache struct {
	values map[string]time.Time
	err    error
}

func newMemoryRevocationCache() *memoryRevocationCache {
	return &memoryRevocationCache{values: map[string]time.Time{}}
}

func (c *memoryRevocationCache) MarkRevoked(_ context.Context, tokenID string, expiresAt time.Time) error {
	if c.err != nil {
		return c.err
	}
	c.values[tokenID] = expiresAt
	return nil
}

func (c *memoryRevocationCache) IsRevoked(_ context.Context, tokenID string) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	expiresAt, ok := c.values[tokenID]
	return ok && time.Now().Before(expiresAt), nil
}
