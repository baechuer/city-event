package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter implements a sliding window rate limiter backed by Redis.
type RedisRateLimiter struct {
	rdb    *redis.Client
	prefix string
}

// NewRedisRateLimiter creates a new Redis-backed rate limiter.
func NewRedisRateLimiter(rdb *redis.Client) *RedisRateLimiter {
	return &RedisRateLimiter{
		rdb:    rdb,
		prefix: "rl:bff:",
	}
}

// RateLimitConfig configures the rate limit for a specific scope.
type RateLimitConfig struct {
	Limit  int           // Max requests allowed
	Window time.Duration // Time window
	KeyFn  func(r *http.Request) string
}

// Middleware returns an HTTP middleware that enforces the rate limit.
func (l *RedisRateLimiter) Middleware(cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l.rdb == nil {
				// Fail open if Redis is not configured
				next.ServeHTTP(w, r)
				return
			}

			key := l.prefix + cfg.KeyFn(r)
			ctx := r.Context()

			allowed, err := l.isAllowed(ctx, key, cfg.Limit, cfg.Window)
			if err != nil {
				// Fail open on Redis errors
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isAllowed checks if the request is within the rate limit using sliding window.
func (l *RedisRateLimiter) isAllowed(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()

	// Lua script for atomic sliding window rate limiting
	// 1. Remove old entries outside the window
	// 2. Count current entries
	// 3. If under limit, add new entry
	// 4. Return count
	script := redis.NewScript(`
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local ttl = tonumber(ARGV[4])
		
		-- Remove old entries
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)
		
		-- Count current entries
		local count = redis.call('ZCARD', key)
		
		if count < limit then
			-- Add new entry with current timestamp as score
			redis.call('ZADD', key, now, now .. '-' .. math.random())
			redis.call('PEXPIRE', key, ttl)
			return 1
		end
		
		return 0
	`)

	result, err := script.Run(ctx, l.rdb, []string{key}, now, windowStart, limit, int(window.Milliseconds())).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// KeyByIP returns the client IP as the rate limit key.
func KeyByIP(r *http.Request) string {
	// Check X-Forwarded-For first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return "ip:" + xff
	}
	return "ip:" + r.RemoteAddr
}

// KeyByUser returns the user ID from context as the rate limit key.
// Falls back to IP if user is not authenticated.
func KeyByUser(r *http.Request) string {
	if userID, ok := r.Context().Value(UserIDKey).(uuid.UUID); ok && userID != uuid.Nil {
		return "user:" + userID.String()
	}
	return KeyByIP(r)
}
