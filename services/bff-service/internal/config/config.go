package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	AuthServiceURL     string
	EventServiceURL    string
	JoinServiceURL     string
	JWTSecret          string
	InternalSecretKey  string
	RLEnabled          bool
	RLLimit            int
	RLWindow           time.Duration
	CORSAllowedOrigins []string
	RedisAddr          string // For distributed rate limiting
}

func Load() *Config {
	return &Config{
		Port:               getEnv("HTTP_PORT", "8080"),
		AuthServiceURL:     getEnv("AUTH_SERVICE_URL", "http://auth-service:8080"),
		EventServiceURL:    getEnv("EVENT_SERVICE_URL", "http://event-service:8080"),
		JoinServiceURL:     getEnv("JOIN_SERVICE_URL", "http://join-service:8080"),
		JWTSecret:          getEnv("JWT_SECRET", "change-me-secret"),
		InternalSecretKey:  getEnv("INTERNAL_SECRET_KEY", "sharedkey"),
		RLEnabled:          getEnvBool("RATE_LIMIT_ENABLED", true),
		RLLimit:            getEnvInt("RATE_LIMIT_REQUESTS", 100),
		RLWindow:           getEnvDuration("RATE_LIMIT_WINDOW", "1m"),
		CORSAllowedOrigins: strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "*"), ","),
		RedisAddr:          getEnv("REDIS_ADDR", ""), // Empty means use in-memory fallback
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func getEnvDuration(key, fallback string) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		v = fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 1 * time.Minute
	}
	return d
}
