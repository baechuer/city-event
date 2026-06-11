package config

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Service.Name != "auth-service" {
		t.Fatalf("unexpected service name %q", cfg.Service.Name)
	}
	if cfg.Environment != "local" {
		t.Fatalf("unexpected environment %q", cfg.Environment)
	}
	if cfg.HTTPAddr != ":8081" {
		t.Fatalf("unexpected http addr %q", cfg.HTTPAddr)
	}
	if cfg.PostgresURL == "" || cfg.RabbitMQURL == "" || cfg.RedisURL == "" {
		t.Fatalf("expected dependency defaults to be set")
	}
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Fatalf("access token ttl = %s, want 15m", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 720*time.Hour {
		t.Fatalf("refresh token ttl = %s, want 720h", cfg.RefreshTokenTTL)
	}
	if cfg.RefreshCookieSecure {
		t.Fatalf("refresh cookie should not be secure by default for local HTTP")
	}
	if !cfg.TokenRevocationCacheEnabled {
		t.Fatalf("token revocation cache should be enabled by default")
	}
	if !cfg.RateLimitEnabled || cfg.RateLimitBackend != "memory" || cfg.RateLimitWindow != time.Minute || cfg.RateLimitRedisTimeout != 200*time.Millisecond || !cfg.RateLimitRedisFailOpen || cfg.RateLimitRequests != 600 || cfg.RateLimitAuthRequests != 60 || cfg.RateLimitMutationRequests != 240 {
		t.Fatalf("unexpected rate limit defaults: enabled=%v backend=%s window=%s redisTimeout=%s failOpen=%v requests=%d auth=%d mutation=%d", cfg.RateLimitEnabled, cfg.RateLimitBackend, cfg.RateLimitWindow, cfg.RateLimitRedisTimeout, cfg.RateLimitRedisFailOpen, cfg.RateLimitRequests, cfg.RateLimitAuthRequests, cfg.RateLimitMutationRequests)
	}
	if cfg.AuthServiceURL != "http://127.0.0.1:8081" || cfg.EventServiceURL != "http://127.0.0.1:8082" {
		t.Fatalf("expected local service URL defaults, got auth=%q event=%q", cfg.AuthServiceURL, cfg.EventServiceURL)
	}
}

func TestLoadServiceSpecificHTTPAddr(t *testing.T) {
	envs := map[string]string{
		"HTTP_ADDR":                      ":9000",
		"AUTH_SERVICE_HTTP_ADDR":         ":9101",
		"CITYEVENTS_ENV":                 "test",
		"POSTGRES_URL":                   "postgres://test:test@localhost:5432/test?sslmode=disable",
		"RABBITMQ_URL":                   "amqp://test:test@localhost:5672/",
		"REDIS_URL":                      "redis://localhost:6379/1",
		"MINIO_ENDPOINT":                 "http://localhost:9000",
		"SMTP_ADDR":                      "localhost:1025",
		"SHUTDOWN_TIMEOUT":               "5s",
		"ACCESS_TOKEN_TTL":               "10m",
		"REFRESH_TOKEN_TTL":              "168h",
		"REFRESH_COOKIE_SECURE":          "true",
		"TOKEN_REVOCATION_CACHE_ENABLED": "false",
		"RATE_LIMIT_ENABLED":             "true",
		"RATE_LIMIT_BACKEND":             "redis",
		"RATE_LIMIT_WINDOW":              "30s",
		"RATE_LIMIT_REDIS_TIMEOUT":       "50ms",
		"RATE_LIMIT_REDIS_FAIL_OPEN":     "false",
		"RATE_LIMIT_REQUESTS":            "50",
		"RATE_LIMIT_AUTH_REQUESTS":       "5",
		"RATE_LIMIT_MUTATION_REQUESTS":   "20",
		"CORS_ALLOWED_ORIGINS":           "https://cityevents.example,http://localhost:18088",
		"UNRELATED_SERVICE_HTTP_ADDR":    ":9200",
	}

	cfg, err := Load("auth-service", mapGetenv(envs))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPAddr != ":9101" {
		t.Fatalf("expected service-specific addr, got %q", cfg.HTTPAddr)
	}
	if cfg.Environment != "test" {
		t.Fatalf("expected test environment, got %q", cfg.Environment)
	}
	if cfg.AccessTokenTTL != 10*time.Minute || cfg.RefreshTokenTTL != 168*time.Hour || !cfg.RefreshCookieSecure {
		t.Fatalf("unexpected token config: access=%s refresh=%s secure=%v", cfg.AccessTokenTTL, cfg.RefreshTokenTTL, cfg.RefreshCookieSecure)
	}
	if cfg.TokenRevocationCacheEnabled {
		t.Fatalf("expected token revocation cache to be disabled")
	}
	if cfg.RateLimitBackend != "redis" || cfg.RateLimitWindow != 30*time.Second || cfg.RateLimitRedisTimeout != 50*time.Millisecond || cfg.RateLimitRedisFailOpen || cfg.RateLimitRequests != 50 || cfg.RateLimitAuthRequests != 5 || cfg.RateLimitMutationRequests != 20 {
		t.Fatalf("unexpected rate limits: backend=%s window=%s redisTimeout=%s failOpen=%v requests=%d auth=%d mutation=%d", cfg.RateLimitBackend, cfg.RateLimitWindow, cfg.RateLimitRedisTimeout, cfg.RateLimitRedisFailOpen, cfg.RateLimitRequests, cfg.RateLimitAuthRequests, cfg.RateLimitMutationRequests)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "https://cityevents.example" {
		t.Fatalf("unexpected allowed origins: %+v", cfg.AllowedOrigins)
	}
}

func TestLoadRejectsUnknownService(t *testing.T) {
	_, err := Load("unknown-service", nil)
	if err == nil {
		t.Fatalf("expected unknown service error")
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	_, err := Load("auth-service", mapGetenv(map[string]string{
		"SHUTDOWN_TIMEOUT": "not-a-duration",
	}))
	if err == nil {
		t.Fatalf("expected invalid timeout error")
	}
}

func TestLoadRejectsInvalidRefreshConfig(t *testing.T) {
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"REFRESH_TOKEN_TTL": "not-a-duration",
	})); err == nil {
		t.Fatalf("expected invalid refresh ttl error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"REFRESH_COOKIE_SECURE": "not-a-bool",
	})); err == nil {
		t.Fatalf("expected invalid refresh cookie secure error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"TOKEN_REVOCATION_CACHE_ENABLED": "not-a-bool",
	})); err == nil {
		t.Fatalf("expected invalid token revocation cache setting error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"RATE_LIMIT_ENABLED": "not-a-bool",
	})); err == nil {
		t.Fatalf("expected invalid rate limit enabled error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"RATE_LIMIT_WINDOW": "not-a-duration",
	})); err == nil {
		t.Fatalf("expected invalid rate limit window error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"RATE_LIMIT_REDIS_TIMEOUT": "not-a-duration",
	})); err == nil {
		t.Fatalf("expected invalid rate limit redis timeout error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"RATE_LIMIT_REDIS_FAIL_OPEN": "not-a-bool",
	})); err == nil {
		t.Fatalf("expected invalid rate limit redis fail-open error")
	}
	if _, err := Load("auth-service", mapGetenv(map[string]string{
		"RATE_LIMIT_REQUESTS": "not-a-number",
	})); err == nil {
		t.Fatalf("expected invalid rate limit request count error")
	}
}

func TestValidateRejectsEmptyEnvironment(t *testing.T) {
	cfg, err := Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Environment = ""

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected empty environment to be rejected")
	}
}

func TestValidateRejectsInvalidHTTPAddr(t *testing.T) {
	cfg, err := Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.HTTPAddr = "not an address"

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid http address to be rejected")
	}
}

func TestValidateRejectsNonPositiveShutdownTimeout(t *testing.T) {
	cfg, err := Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.ShutdownTimeout = 0 * time.Second

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected non-positive shutdown timeout to be rejected")
	}
}

func TestValidateRejectsNonPositiveRateLimit(t *testing.T) {
	cfg, err := Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.RateLimitRequests = 0

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected non-positive rate limit to be rejected")
	}
}

func TestValidateRejectsInvalidRateLimitBackend(t *testing.T) {
	_, err := Load("auth-service", mapGetenv(map[string]string{
		"RATE_LIMIT_BACKEND": "unknown",
	}))
	if err == nil {
		t.Fatalf("expected invalid rate limit backend")
	}
}

func TestValidateRejectsNonPositiveRedisRateLimitTimeout(t *testing.T) {
	cfg, err := Load("auth-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.RateLimitRedisTimeout = 0

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected non-positive redis rate limit timeout to be rejected")
	}
}

func TestLoadGatewayDownstreamURLs(t *testing.T) {
	cfg, err := Load("api-gateway", mapGetenv(map[string]string{
		"AUTH_SERVICE_URL":  "http://auth-service",
		"EVENT_SERVICE_URL": "http://event-registration-service",
		"FEED_SERVICE_URL":  "http://feed-service",
		"MEDIA_SERVICE_URL": "http://media-service",
	}))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AuthServiceURL != "http://auth-service" || cfg.MediaServiceURL != "http://media-service" {
		t.Fatalf("unexpected downstream urls: %+v", cfg)
	}
}

func TestValidateRejectsInvalidDownstreamURL(t *testing.T) {
	_, err := Load("api-gateway", mapGetenv(map[string]string{
		"AUTH_SERVICE_URL": "not-a-url",
	}))
	if err == nil {
		t.Fatalf("expected invalid downstream url")
	}
}

func TestServiceCatalogHasUniqueNamesAndPorts(t *testing.T) {
	names := map[string]bool{}
	ports := map[string]bool{}

	for _, service := range Services() {
		if names[service.Name] {
			t.Fatalf("duplicate service name %q", service.Name)
		}
		names[service.Name] = true

		port := strings.TrimPrefix(service.DefaultHTTPAddr, ":")
		if _, err := strconv.Atoi(port); err != nil {
			t.Fatalf("service %s has non-numeric port %q", service.Name, port)
		}
		if ports[service.DefaultHTTPAddr] {
			t.Fatalf("duplicate default http addr %q", service.DefaultHTTPAddr)
		}
		ports[service.DefaultHTTPAddr] = true
	}
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
