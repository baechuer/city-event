package config

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ServiceDefinition struct {
	Name            string
	Role            string
	DefaultHTTPAddr string
}

type Config struct {
	Service         ServiceDefinition
	Environment     string
	HTTPAddr        string
	ShutdownTimeout time.Duration
	PostgresURL     string
	RabbitMQURL     string
	RedisURL        string
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOBucket     string
	SMTPAddr        string
	JWTSecret       string
	JWTIssuer       string
	AccessTokenTTL  time.Duration
}

var serviceDefinitions = []ServiceDefinition{
	{Name: "api-gateway", Role: "gateway", DefaultHTTPAddr: ":8080"},
	{Name: "auth-service", Role: "http-service", DefaultHTTPAddr: ":8081"},
	{Name: "event-registration-service", Role: "http-service", DefaultHTTPAddr: ":8082"},
	{Name: "feed-service", Role: "http-service", DefaultHTTPAddr: ":8083"},
	{Name: "notification-service", Role: "http-service", DefaultHTTPAddr: ":8084"},
	{Name: "media-service", Role: "http-service", DefaultHTTPAddr: ":8085"},
	{Name: "media-worker", Role: "worker", DefaultHTTPAddr: ":8086"},
	{Name: "outbox-relay", Role: "worker", DefaultHTTPAddr: ":8087"},
	{Name: "feed-worker", Role: "worker", DefaultHTTPAddr: ":8088"},
	{Name: "notification-worker", Role: "worker", DefaultHTTPAddr: ":8089"},
}

func Services() []ServiceDefinition {
	out := make([]ServiceDefinition, len(serviceDefinitions))
	copy(out, serviceDefinitions)
	return out
}

func LookupService(name string) (ServiceDefinition, bool) {
	for _, service := range serviceDefinitions {
		if service.Name == name {
			return service, true
		}
	}
	return ServiceDefinition{}, false
}

func Load(serviceName string, getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	service, ok := LookupService(serviceName)
	if !ok {
		return Config{}, fmt.Errorf("unknown service %q", serviceName)
	}

	shutdownTimeout, err := parseDuration(env(getenv, "SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}
	accessTokenTTL, err := parseDuration(env(getenv, "ACCESS_TOKEN_TTL", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL: %w", err)
	}

	cfg := Config{
		Service:         service,
		Environment:     env(getenv, "CITYEVENTS_ENV", "local"),
		HTTPAddr:        serviceEnv(getenv, service.Name, "HTTP_ADDR", env(getenv, "HTTP_ADDR", service.DefaultHTTPAddr)),
		ShutdownTimeout: shutdownTimeout,
		PostgresURL:     serviceEnv(getenv, service.Name, "POSTGRES_URL", env(getenv, "POSTGRES_URL", "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable")),
		RabbitMQURL:     env(getenv, "RABBITMQ_URL", "amqp://cityevents:cityevents@localhost:5672/"),
		RedisURL:        env(getenv, "REDIS_URL", "redis://localhost:6379/0"),
		MinIOEndpoint:   env(getenv, "MINIO_ENDPOINT", "http://localhost:9000"),
		MinIOAccessKey:  env(getenv, "MINIO_ACCESS_KEY", "cityevents"),
		MinIOSecretKey:  env(getenv, "MINIO_SECRET_KEY", "cityevents-password"),
		MinIOBucket:     env(getenv, "MINIO_BUCKET", "cityevents-media"),
		SMTPAddr:        env(getenv, "SMTP_ADDR", "localhost:1025"),
		JWTSecret:       serviceEnv(getenv, service.Name, "JWT_SECRET", env(getenv, "JWT_SECRET", "dev-secret-change-me")),
		JWTIssuer:       env(getenv, "JWT_ISSUER", "cityevents"),
		AccessTokenTTL:  accessTokenTTL,
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Service.Name == "" {
		return fmt.Errorf("service name is required")
	}
	if c.Environment == "" {
		return fmt.Errorf("environment is required")
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("http address is required")
	}
	if _, _, err := net.SplitHostPort(normalizeAddr(c.HTTPAddr)); err != nil {
		return fmt.Errorf("invalid http address %q: %w", c.HTTPAddr, err)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("jwt secret is required")
	}
	if c.JWTIssuer == "" {
		return fmt.Errorf("jwt issuer is required")
	}
	if c.AccessTokenTTL <= 0 {
		return fmt.Errorf("access token ttl must be positive")
	}
	if c.MinIOEndpoint == "" {
		return fmt.Errorf("minio endpoint is required")
	}
	if c.MinIOAccessKey == "" {
		return fmt.Errorf("minio access key is required")
	}
	if c.MinIOSecretKey == "" {
		return fmt.Errorf("minio secret key is required")
	}
	if c.MinIOBucket == "" {
		return fmt.Errorf("minio bucket is required")
	}
	return nil
}

func ServiceNames() []string {
	names := make([]string, 0, len(serviceDefinitions))
	for _, service := range serviceDefinitions {
		names = append(names, service.Name)
	}
	sort.Strings(names)
	return names
}

func serviceEnv(getenv func(string) string, serviceName, key, fallback string) string {
	name := strings.ToUpper(strings.ReplaceAll(serviceName, "-", "_")) + "_" + key
	return env(getenv, name, fallback)
}

func env(getenv func(string) string, key, fallback string) string {
	if value := strings.TrimSpace(getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseDuration(value string) (time.Duration, error) {
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}
	return time.ParseDuration(value)
}

func normalizeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
