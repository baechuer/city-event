package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const cacheTTL = 30 * time.Second

type Cache interface {
	Get(context.Context, string, any) (bool, error)
	Set(context.Context, string, any, time.Duration) error
}

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func NewRedisClient(redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return redis.NewClient(options), nil
}

func (c *RedisCache) Get(ctx context.Context, key string, target any) (bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, err
	}
	return true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, raw, ttl).Err()
}

func listCacheKey(query Query) string {
	query = NormalizeQuery(query)
	city := url.QueryEscape(strings.ToLower(query.City))
	return fmt.Sprintf("feed:v1:list:city=%s:limit=%d:offset=%d", city, query.Limit, query.Offset)
}

func detailCacheKey(eventID string) string {
	return "feed:v1:detail:" + url.PathEscape(strings.TrimSpace(eventID))
}

type MemoryCache struct {
	values map[string][]byte
	err    error
	gets   int
	sets   int
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{values: map[string][]byte{}}
}

func (c *MemoryCache) Get(_ context.Context, key string, target any) (bool, error) {
	c.gets++
	if c.err != nil {
		return false, c.err
	}
	raw, ok := c.values[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, err
	}
	return true, nil
}

func (c *MemoryCache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	c.sets++
	if c.err != nil {
		return c.err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.values[key] = raw
	return nil
}
