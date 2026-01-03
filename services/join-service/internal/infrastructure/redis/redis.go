// PATH: services/join-service/internal/infrastructure/redis/redis.go
package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Cache struct {
	Client *redis.Client
}

func New(addr, pass string, db int) *Cache {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr, Password: pass, DB: db,
	})
	return &Cache{Client: rdb}
}

func (c *Cache) GetEventCapacity(ctx context.Context, eventID uuid.UUID) (int, error) {
	val, err := c.Client.Get(ctx, "event:cap:"+eventID.String()).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, domain.ErrCacheMiss
		}
		return 0, err
	}
	return strconv.Atoi(val)
}

func (c *Cache) SetEventCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error {
	return c.Client.Set(ctx, "event:cap:"+eventID.String(), capacity, 24*time.Hour).Err()
}

// AllowRequest: Simple Fixed Window Rate Limit
func (c *Cache) AllowRequest(ctx context.Context, ip string, limit int, window time.Duration) (bool, error) {
	key := "ratelimit:" + ip
	count, err := c.Client.Incr(ctx, key).Result()
	if err != nil {
		return true, nil // fail open
	}
	if count == 1 {
		_ = c.Client.Expire(ctx, key, window).Err()
	}
	return count <= int64(limit), nil
}

// Ping checks Redis connectivity for readiness probes
func (c *Cache) Ping(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}
