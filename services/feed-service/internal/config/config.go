package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env      string
	HTTPAddr string

	// Database
	DBAddr string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// RabbitMQ
	RabbitURL string

	// Security
	AnonCookieSecret string
	AnonCookieTTL    time.Duration

	// Rate limiting
	RateLimitPerActor int
	RateLimitPerIP    int
}

func Load() (*Config, error) {
	return &Config{
		Env:               getEnv("APP_ENV", "dev"),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8084"),
		DBAddr:            getEnv("DB_ADDR", "postgres://user:pass@localhost:5432/feed?sslmode=disable"),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           getEnvInt("REDIS_DB", 0),
		RabbitURL:         getEnv("RABBIT_URL", "amqp://guest:guest@localhost:5672/"),
		AnonCookieSecret:  getEnv("ANON_COOKIE_SECRET", "dev-secret-change-in-prod"),
		AnonCookieTTL:     time.Duration(getEnvInt("ANON_COOKIE_TTL_DAYS", 365)) * 24 * time.Hour,
		RateLimitPerActor: getEnvInt("RATE_LIMIT_PER_ACTOR", 10),
		RateLimitPerIP:    getEnvInt("RATE_LIMIT_PER_IP", 100),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
