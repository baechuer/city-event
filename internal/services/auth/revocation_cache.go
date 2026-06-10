package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const revokedTokenCachePrefix = "auth:revoked:jti:"

type TokenRevocationCache interface {
	MarkRevoked(context.Context, string, time.Time) error
	IsRevoked(context.Context, string) (bool, error)
}

type CachedRevocationRepository struct {
	Repository
	cache TokenRevocationCache
}

func WithRevocationCache(repo Repository, cache TokenRevocationCache) Repository {
	if repo == nil || cache == nil {
		return repo
	}
	return &CachedRevocationRepository{Repository: repo, cache: cache}
}

func (r *CachedRevocationRepository) RevokeToken(ctx context.Context, tokenID, userID string, expiresAt time.Time) error {
	if err := r.Repository.RevokeToken(ctx, tokenID, userID, expiresAt); err != nil {
		return err
	}
	_ = r.cache.MarkRevoked(ctx, tokenID, expiresAt)
	return nil
}

func (r *CachedRevocationRepository) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	revoked, err := r.cache.IsRevoked(ctx, tokenID)
	if err == nil && revoked {
		return true, nil
	}

	revoked, err = r.Repository.IsTokenRevoked(ctx, tokenID)
	if err != nil {
		return false, err
	}
	if revoked {
		_ = r.cache.MarkRevoked(ctx, tokenID, time.Now().Add(time.Minute))
	}
	return revoked, nil
}

type RedisRevocationCache struct {
	client *redis.Client
	now    func() time.Time
}

func NewRedisRevocationCache(client *redis.Client) *RedisRevocationCache {
	return &RedisRevocationCache{client: client, now: time.Now}
}

func NewRedisRevocationCacheFromURL(ctx context.Context, redisURL string) (*RedisRevocationCache, func() error, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, nil, err
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return NewRedisRevocationCache(client), client.Close, nil
}

func (c *RedisRevocationCache) MarkRevoked(ctx context.Context, tokenID string, expiresAt time.Time) error {
	if c == nil || c.client == nil || tokenID == "" {
		return nil
	}
	ttl := time.Until(expiresAt)
	if c.now != nil {
		ttl = expiresAt.Sub(c.now())
	}
	if ttl <= 0 {
		return nil
	}
	return c.client.Set(ctx, revokedTokenCachePrefix+tokenID, "1", ttl).Err()
}

func (c *RedisRevocationCache) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	if c == nil || c.client == nil || tokenID == "" {
		return false, nil
	}
	count, err := c.client.Exists(ctx, revokedTokenCachePrefix+tokenID).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
