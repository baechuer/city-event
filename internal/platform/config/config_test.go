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
	if cfg.AuthServiceURL != "http://127.0.0.1:8081" || cfg.EventServiceURL != "http://127.0.0.1:8082" {
		t.Fatalf("expected local service URL defaults, got auth=%q event=%q", cfg.AuthServiceURL, cfg.EventServiceURL)
	}
}

func TestLoadServiceSpecificHTTPAddr(t *testing.T) {
	envs := map[string]string{
		"HTTP_ADDR":                   ":9000",
		"AUTH_SERVICE_HTTP_ADDR":      ":9101",
		"CITYEVENTS_ENV":              "test",
		"POSTGRES_URL":                "postgres://test:test@localhost:5432/test?sslmode=disable",
		"RABBITMQ_URL":                "amqp://test:test@localhost:5672/",
		"REDIS_URL":                   "redis://localhost:6379/1",
		"MINIO_ENDPOINT":              "http://localhost:9000",
		"SMTP_ADDR":                   "localhost:1025",
		"SHUTDOWN_TIMEOUT":            "5s",
		"UNRELATED_SERVICE_HTTP_ADDR": ":9200",
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
