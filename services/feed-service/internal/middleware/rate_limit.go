package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	limit    int
	window   time.Duration
	cleanupC <-chan time.Time
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		limit:    limit,
		window:   window,
		cleanupC: time.Tick(5 * time.Minute),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok || now.Sub(b.lastReset) > rl.window {
		rl.buckets[key] = &bucket{tokens: rl.limit - 1, lastReset: now}
		return true
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

func (rl *RateLimiter) cleanup() {
	for range rl.cleanupC {
		rl.mu.Lock()
		now := time.Now()
		for k, b := range rl.buckets {
			if now.Sub(b.lastReset) > 2*rl.window {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit middleware
func RateLimit(actorLimiter, ipLimiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check IP limit
			ip := r.RemoteAddr
			if !ipLimiter.Allow(ip) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Check actor limit if present
			if anonID, ok := AnonIDFromContext(r.Context()); ok {
				actorKey := "a:" + anonID
				// Also check user_id if authenticated
				if userID := r.Header.Get("X-User-ID"); userID != "" {
					actorKey = "u:" + userID
				}
				if !actorLimiter.Allow(actorKey) {
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
