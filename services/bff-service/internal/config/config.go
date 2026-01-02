package config

import (
	"os"
)

type Config struct {
	Port            string
	AuthServiceURL  string
	EventServiceURL string
	JoinServiceURL  string
	JWTSecret       string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("HTTP_PORT", "8080"),
		AuthServiceURL:  getEnv("AUTH_SERVICE_URL", "http://auth-service:8080"),
		EventServiceURL: getEnv("EVENT_SERVICE_URL", "http://event-service:8080"),
		JoinServiceURL:  getEnv("JOIN_SERVICE_URL", "http://join-service:8080"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me-secret"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
