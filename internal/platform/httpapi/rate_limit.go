package httpapi

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/observability"
)

type rateLimitDecision struct {
	allowed    bool
	limit      int
	retryAfter time.Duration
	scope      string
}

type rateLimitBucket struct {
	start time.Time
	count int
}

type fixedWindowRateLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	window  time.Duration
	buckets map[string]rateLimitBucket
}

func newFixedWindowRateLimiter(window time.Duration) *fixedWindowRateLimiter {
	return &fixedWindowRateLimiter{
		now:     time.Now,
		window:  window,
		buckets: map[string]rateLimitBucket{},
	}
}

func (l *fixedWindowRateLimiter) allow(key string, limit int) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket := l.buckets[key]
	if bucket.start.IsZero() || now.Sub(bucket.start) >= l.window {
		bucket = rateLimitBucket{start: now}
	}
	bucket.count++
	l.buckets[key] = bucket

	if bucket.count <= limit {
		return true, 0
	}
	retryAfter := bucket.start.Add(l.window).Sub(now)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}

func rateLimitMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	limiter := newFixedWindowRateLimiter(cfg.RateLimitWindow)
	return func(next http.Handler) http.Handler {
		if !cfg.RateLimitEnabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipRateLimit(r) {
				next.ServeHTTP(w, r)
				return
			}

			decision := rateLimitDecisionFor(cfg, limiter, r)
			if decision.allowed {
				next.ServeHTTP(w, r)
				return
			}

			observability.RecordRateLimitedRequest(cfg.Service.Name, decision.scope)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", retryAfterSeconds(decision.retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"code":    "rate_limited",
					"message": "too many requests; retry later",
				},
			})
		})
	}
}

func rateLimitDecisionFor(cfg config.Config, limiter *fixedWindowRateLimiter, r *http.Request) rateLimitDecision {
	scope, limit := rateLimitScope(cfg, r)
	key := strings.Join([]string{clientAddress(r), scope, r.Method, r.URL.Path}, "|")
	allowed, retryAfter := limiter.allow(key, limit)
	return rateLimitDecision{allowed: allowed, limit: limit, retryAfter: retryAfter, scope: scope}
}

func rateLimitScope(cfg config.Config, r *http.Request) (string, int) {
	path := strings.ToLower(r.URL.Path)
	if r.Method == http.MethodPost && strings.HasPrefix(path, "/v1/auth/") {
		return "auth", cfg.RateLimitAuthRequests
	}
	if r.Method == http.MethodPatch || r.Method == http.MethodDelete || r.Method == http.MethodPost {
		return "mutation", cfg.RateLimitMutationRequests
	}
	return "read", cfg.RateLimitRequests
}

func skipRateLimit(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	switch r.URL.Path {
	case "/", "/livez", "/readyz", "/metrics":
		return true
	default:
		return false
	}
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func retryAfterSeconds(duration time.Duration) string {
	seconds := int(math.Ceil(duration.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
