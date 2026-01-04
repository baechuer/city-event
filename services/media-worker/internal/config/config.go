package config

import (
	"os"
	"strconv"
)

// Config holds configuration for the media worker.
type Config struct {
	// Database
	DatabaseURL string

	// S3/MinIO
	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3UsePathStyle    bool
	RawBucket         string
	PublicBucket      string

	// RabbitMQ
	RabbitURL      string
	RabbitExchange string
	RabbitQueue    string

	// Processing limits
	MaxUploadSize  int64
	MaxImageWidth  int
	MaxImageHeight int
}

// Load loads configuration from environment variables.
func Load() *Config {
	return &Config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/media_db?sslmode=disable"),
		S3Endpoint:        getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", "minioadmin"),
		S3SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", "minioadmin"),
		S3Region:          getEnv("S3_REGION", "us-east-1"),
		S3UsePathStyle:    getEnvBool("S3_USE_PATH_STYLE", true),
		RawBucket:         getEnv("S3_RAW_BUCKET", "raw"),
		PublicBucket:      getEnv("S3_PUBLIC_BUCKET", "public"),
		RabbitURL:         getEnv("RABBIT_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitExchange:    getEnv("RABBIT_EXCHANGE", "city.events"),
		RabbitQueue:       getEnv("RABBIT_QUEUE", "media-worker.q"),
		MaxUploadSize:     getEnvInt64("MAX_UPLOAD_SIZE", 10*1024*1024),
		MaxImageWidth:     getEnvInt("MAX_IMAGE_WIDTH", 8000),
		MaxImageHeight:    getEnvInt("MAX_IMAGE_HEIGHT", 8000),
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
		b, _ := strconv.ParseBool(v)
		return b
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		i, _ := strconv.Atoi(v)
		return i
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if v := os.Getenv(key); v != "" {
		i, _ := strconv.ParseInt(v, 10, 64)
		return i
	}
	return defaultVal
}
