package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the media service.
type Config struct {
	// HTTP server
	HTTPAddr string

	// Database
	DatabaseURL string

	// S3/MinIO
	S3Endpoint         string
	S3ExternalEndpoint string // External endpoint for browser access (e.g. http://localhost:9000)
	S3AccessKeyID      string
	S3SecretAccessKey  string
	S3Region           string
	S3UsePathStyle     bool // true for MinIO, false for real S3
	RawBucket          string
	PublicBucket       string
	CDNBaseURL         string // base URL for public images

	// RabbitMQ
	RabbitURL      string
	RabbitExchange string

	// Upload constraints
	MaxUploadSize  int64         // bytes
	MaxImageWidth  int           // pixels
	MaxImageHeight int           // pixels
	PresignTTL     time.Duration // presigned URL validity
	AllowedMIME    []string
}

// Load loads configuration from environment variables.
func Load() *Config {
	return &Config{
		HTTPAddr:           getEnv("HTTP_ADDR", ":8085"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/media_db?sslmode=disable"),
		S3Endpoint:         getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3ExternalEndpoint: getEnv("S3_EXTERNAL_ENDPOINT", "http://localhost:9000"),
		S3AccessKeyID:      getEnv("S3_ACCESS_KEY_ID", "minioadmin"),
		S3SecretAccessKey:  getEnv("S3_SECRET_ACCESS_KEY", "minioadmin"),
		S3Region:           getEnv("S3_REGION", "us-east-1"),
		S3UsePathStyle:     getEnvBool("S3_USE_PATH_STYLE", true), // true for MinIO
		RawBucket:          getEnv("S3_RAW_BUCKET", "raw"),
		PublicBucket:       getEnv("S3_PUBLIC_BUCKET", "public"),
		CDNBaseURL:         getEnv("CDN_BASE_URL", "http://localhost:9000/public"),
		RabbitURL:          getEnv("RABBIT_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitExchange:     getEnv("RABBIT_EXCHANGE", "city.events"),
		MaxUploadSize:      getEnvInt64("MAX_UPLOAD_SIZE", 10*1024*1024), // 10MB
		MaxImageWidth:      getEnvInt("MAX_IMAGE_WIDTH", 8000),
		MaxImageHeight:     getEnvInt("MAX_IMAGE_HEIGHT", 8000),
		PresignTTL:         getEnvDuration("PRESIGN_TTL", 5*time.Minute),
		AllowedMIME:        []string{"image/jpeg", "image/png", "image/webp"},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return defaultVal
}
