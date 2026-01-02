package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env string

	// RabbitMQ
	RabbitURL    string
	Exchange     string
	Queue        string
	BindKeysCSV  string
	Prefetch     int
	ConsumeTag   string
	ShutdownWait time.Duration

	// Email / SMTP
	EmailSender string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPTimeout  time.Duration
	SMTPInsecure bool // NEW for dev/it

	// Email-service Web
	EmailWebAddr       string
	EmailPublicBaseURL string
	AuthBaseURL        string
	AuthInternalSecret string

	// Redis
	RedisEnabled  bool
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	EmailIdempotencyTTL time.Duration

	// ---- NEW: HTTP/API Rate Limiting ----
	RLEnabled     bool
	RLIPLimit     int
	RLIPWindow    time.Duration
	RLTokenLimit  int
	RLTokenWindow time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}

	cfg.Env = getEnvFirst([]string{"APP_ENV", "ENV"}, "dev")

	cfg.RabbitURL = strings.TrimSpace(os.Getenv("RABBIT_URL"))
	if cfg.RabbitURL == "" {
		return nil, fmt.Errorf("missing required env var: RABBIT_URL")
	}

	cfg.Exchange = getEnv("RABBIT_EXCHANGE", "city.events")
	cfg.Queue = getEnv("RABBIT_QUEUE", "email-service.q")
	cfg.BindKeysCSV = getEnv("RABBIT_BIND_KEYS", "auth.email.#,auth.password.#,email.#")

	cfg.Prefetch = getInt("RABBIT_PREFETCH", 10)
	cfg.ConsumeTag = getEnv("RABBIT_CONSUMER_TAG", "email-service")
	cfg.ShutdownWait = getDuration("SHUTDOWN_WAIT", 10*time.Second)

	cfg.EmailSender = getEnv("EMAIL_SENDER", "fake")

	cfg.SMTPHost = getEnv("SMTP_HOST", "")
	cfg.SMTPPort = getInt("SMTP_PORT", 587)
	cfg.SMTPUsername = getEnv("SMTP_USERNAME", "")
	cfg.SMTPPassword = getEnv("SMTP_PASSWORD", "")
	cfg.SMTPFrom = getEnv("SMTP_FROM", cfg.SMTPUsername)
	cfg.SMTPTimeout = getDuration("SMTP_TIMEOUT", 10*time.Second)
	cfg.SMTPInsecure = getBool("SMTP_INSECURE", false)

	if cfg.EmailSender == "smtp" {
		if cfg.SMTPHost == "" {
			return nil, fmt.Errorf("smtp sender selected but missing SMTP_HOST")
		}
	}

	// Web defaults
	cfg.EmailWebAddr = getEnv("EMAIL_WEB_ADDR", ":8090")
	cfg.EmailPublicBaseURL = strings.TrimRight(getEnv("EMAIL_PUBLIC_BASE_URL", "http://localhost:8090"), "/")
	cfg.AuthBaseURL = strings.TrimRight(getEnv("AUTH_BASE_URL", "http://localhost:8080"), "/")
	cfg.AuthInternalSecret = getEnv("INTERNAL_SECRET_KEY", "dev-secret-key")

	// Redis
	cfg.RedisEnabled = getBool("REDIS_ENABLED", false)
	cfg.RedisAddr = getEnv("REDIS_ADDR", "localhost:6379")
	cfg.RedisPassword = getEnv("REDIS_PASSWORD", "")
	cfg.RedisDB = getInt("REDIS_DB", 0)

	cfg.EmailIdempotencyTTL = getDuration("EMAIL_IDEMPOTENCY_TTL", 24*time.Hour)

	// ---- Rate limiting defaults ----
	cfg.RLEnabled = getBool("RL_ENABLED", false)
	cfg.RLIPLimit = getInt("RL_IP_LIMIT", 30)
	cfg.RLIPWindow = getDuration("RL_IP_WINDOW", 1*time.Minute)
	cfg.RLTokenLimit = getInt("RL_TOKEN_LIMIT", 10)
	cfg.RLTokenWindow = getDuration("RL_TOKEN_WINDOW", 10*time.Minute)

	// Guard: prevent the classic "REDIS_ADDR=localhost:6379 OTHER=..." parsing issue
	if strings.Contains(cfg.RedisAddr, " ") {
		return nil, fmt.Errorf("bad REDIS_ADDR (contains spaces): %q", cfg.RedisAddr)
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getEnvFirst(keys []string, def string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return def
}

func getInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n := def
	_, _ = fmt.Sscanf(v, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}

func getDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}
