package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/baechuer/cityevents/internal/platform/authn"
	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/redis/go-redis/v9"
)

type rateLimitDecision struct {
	allowed    bool
	limit      int
	retryAfter time.Duration
	scope      string
	storeErr   error
}

type rateLimitBucket struct {
	start time.Time
	count int
}

type rateLimitStore interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error)
}

type memoryFixedWindowRateLimitStore struct {
	mu      sync.Mutex
	now     func() time.Time
	buckets map[string]rateLimitBucket
}

func newMemoryFixedWindowRateLimitStore() *memoryFixedWindowRateLimitStore {
	return &memoryFixedWindowRateLimitStore{
		now:     time.Now,
		buckets: map[string]rateLimitBucket{},
	}
}

func (s *memoryFixedWindowRateLimitStore) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if window <= 0 {
		return false, 0, fmt.Errorf("rate limit window must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	bucket := s.buckets[key]
	if bucket.start.IsZero() || now.Sub(bucket.start) >= window {
		bucket = rateLimitBucket{start: now}
	}
	bucket.count++
	s.buckets[key] = bucket

	if bucket.count <= limit {
		return true, 0, nil
	}
	retryAfter := bucket.start.Add(window).Sub(now)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter, nil
}

type redisRateLimitStore struct {
	client  *redis.Client
	timeout time.Duration
}

var redisRateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return {current, ttl}
`)

func newRedisRateLimitStore(redisURL string, timeout time.Duration) (rateLimitStore, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &redisRateLimitStore{client: redis.NewClient(options), timeout: timeout}, nil
}

func (s *redisRateLimitStore) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if window <= 0 {
		return false, 0, fmt.Errorf("rate limit window must be positive")
	}
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := redisRateLimitScript.Run(ctx, s.client, []string{key}, window.Milliseconds()).Result()
	if err != nil {
		return false, 0, err
	}
	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected redis rate limit response %T", result)
	}
	count, ok := redisScriptInt(values[0])
	if !ok {
		return false, 0, fmt.Errorf("unexpected redis rate limit count %T", values[0])
	}
	ttlMillis, ok := redisScriptInt(values[1])
	if !ok {
		return false, 0, fmt.Errorf("unexpected redis rate limit ttl %T", values[1])
	}

	if count <= int64(limit) {
		return true, 0, nil
	}
	retryAfter := time.Duration(ttlMillis) * time.Millisecond
	if ttlMillis < 0 {
		retryAfter = window
	}
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter, nil
}

func redisScriptInt(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

type unavailableRateLimitStore struct {
	err error
}

func (s unavailableRateLimitStore) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 0, s.err
}

func newRateLimitStore(cfg config.Config) rateLimitStore {
	switch cfg.RateLimitBackend {
	case "redis":
		store, err := newRedisRateLimitStore(cfg.RedisURL, cfg.RateLimitRedisTimeout)
		if err != nil {
			return unavailableRateLimitStore{err: err}
		}
		return store
	default:
		return newMemoryFixedWindowRateLimitStore()
	}
}

func rateLimitMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return rateLimitMiddlewareWithStore(cfg, newRateLimitStore(cfg))
}

func rateLimitMiddlewareWithStore(cfg config.Config, store rateLimitStore) func(http.Handler) http.Handler {
	if store == nil {
		store = newMemoryFixedWindowRateLimitStore()
	}
	return func(next http.Handler) http.Handler {
		if !cfg.RateLimitEnabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipRateLimit(r) {
				next.ServeHTTP(w, r)
				return
			}

			decision := rateLimitDecisionFor(cfg, store, r)
			if decision.storeErr != nil {
				mode := rateLimitFailureMode(cfg)
				observability.RecordRateLimitStoreError(cfg.Service.Name, cfg.RateLimitBackend, mode)
				if cfg.RateLimitRedisFailOpen {
					next.ServeHTTP(w, r)
					return
				}
				decision.allowed = false
				if decision.retryAfter <= 0 {
					decision.retryAfter = time.Second
				}
			}
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

func rateLimitDecisionFor(cfg config.Config, store rateLimitStore, r *http.Request) rateLimitDecision {
	scope, limit := rateLimitScope(cfg, r)
	key := rateLimitKey(cfg, r, scope)
	allowed, retryAfter, err := store.Allow(r.Context(), key, limit, cfg.RateLimitWindow)
	return rateLimitDecision{allowed: allowed, limit: limit, retryAfter: retryAfter, scope: scope, storeErr: err}
}

func rateLimitKey(cfg config.Config, r *http.Request, scope string) string {
	return strings.Join([]string{
		"cityevents",
		"rate-limit",
		cfg.Environment,
		cfg.Service.Name,
		rateLimitIdentity(cfg, r),
		scope,
		r.Method,
		r.URL.Path,
	}, "|")
}

func rateLimitIdentity(cfg config.Config, r *http.Request) string {
	tokens := authn.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL)
	if claims, ok := tokens.ClaimsFromRequest(r); ok {
		return "user:" + claims.UserID
	}
	return "ip:" + clientAddress(cfg, r)
}

func rateLimitFailureMode(cfg config.Config) string {
	if cfg.RateLimitRedisFailOpen {
		return "fail_open"
	}
	return "fail_closed"
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

func clientAddress(cfg config.Config, r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	remoteIP := net.ParseIP(strings.TrimSpace(host))
	if remoteIP != nil && isTrustedProxy(remoteIP, cfg.TrustedProxyCIDRs) {
		if forwarded := firstForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
	}
	if strings.TrimSpace(host) != "" {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func isTrustedProxy(ip net.IP, cidrs []string) bool {
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func firstForwardedIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func retryAfterSeconds(duration time.Duration) string {
	seconds := int(math.Ceil(duration.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
