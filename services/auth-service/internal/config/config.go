package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// App
	Env string // dev / staging / prod

	// HTTP
	HTTPAddr string

	// Auth / Security
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	InternalSecret  string

	// Infrastructure
	DBAddr        string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RabbitURL     string

	// Cache tuning
	TokenVersionCacheTTL time.Duration

	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPIdleTimeout  time.Duration

	// One-time token flows
	VerifyEmailBaseURL    string
	PasswordResetBaseURL  string
	VerifyEmailTokenTTL   time.Duration
	PasswordResetTokenTTL time.Duration

	// OAuth Providers
	GoogleClientID     string
	GoogleClientSecret string
	OAuthStateTTL      time.Duration // default 10m
	OAuthCallbackURL   string        // e.g., http://localhost:8080/auth/v1/oauth/google/callback
	FrontendOrigin     string        // for postMessage origin validation
	AllowedRedirects   []string      // whitelist for redirect_to

	// Debug toggles
	DBDebug bool
}

func Load() (*Config, error) {
	cfg := &Config{}

	// ✅ Env (support both APP_ENV and ENV)
	cfg.Env = getEnvFirst([]string{"APP_ENV", "ENV"}, "dev")
	cfg.HTTPAddr = getEnv("HTTP_ADDR", ":8080")

	// required values
	cfg.JWTSecret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("missing required env var: JWT_SECRET")
	}

	cfg.InternalSecret = getEnv("INTERNAL_SECRET_KEY", "dev-secret-key")
	if cfg.Env == "prod" && cfg.InternalSecret == "dev-secret-key" {
		return nil, fmt.Errorf("INTERNAL_SECRET_KEY must be set in prod")
	}

	// optional with defaults
	var err error
	cfg.AccessTokenTTL, err = getDuration("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	cfg.RefreshTokenTTL, err = getDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	// One-time token URLs (required)
	cfg.VerifyEmailBaseURL = strings.TrimSpace(os.Getenv("VERIFY_EMAIL_BASE_URL"))
	if cfg.VerifyEmailBaseURL == "" {
		return nil, fmt.Errorf("missing required env var: VERIFY_EMAIL_BASE_URL")
	}
	if !strings.Contains(cfg.VerifyEmailBaseURL, "token=") {
		return nil, fmt.Errorf("VERIFY_EMAIL_BASE_URL must contain `token=`")
	}

	cfg.PasswordResetBaseURL = strings.TrimSpace(os.Getenv("PASSWORD_RESET_BASE_URL"))
	if cfg.PasswordResetBaseURL == "" {
		return nil, fmt.Errorf("missing required env var: PASSWORD_RESET_BASE_URL")
	}
	if !strings.Contains(cfg.PasswordResetBaseURL, "token=") {
		return nil, fmt.Errorf("PASSWORD_RESET_BASE_URL must contain `token=`")
	}

	cfg.VerifyEmailTokenTTL, err = getDuration("VERIFY_EMAIL_TOKEN_TTL", 24*time.Hour)
	if err != nil {
		return nil, err
	}
	cfg.PasswordResetTokenTTL, err = getDuration("PASSWORD_RESET_TOKEN_TTL", 30*time.Minute)
	if err != nil {
		return nil, err
	}

	// Infrastructure DSNs (required)
	cfg.DBAddr = strings.TrimSpace(os.Getenv("DB_ADDR"))
	if cfg.DBAddr == "" {
		return nil, fmt.Errorf("missing required env var: DB_ADDR")
	}
	// ✅ Basic DSN sanity check (catches \r and broken url)
	if err := validatePostgresDSN(cfg.DBAddr); err != nil {
		return nil, fmt.Errorf("invalid DB_ADDR: %w", err)
	}

	// ✅ Redis (addr required in your current design)
	cfg.RedisAddr = strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if cfg.RedisAddr == "" {
		return nil, fmt.Errorf("missing required env var: REDIS_ADDR")
	}
	cfg.RedisPassword = strings.TrimSpace(os.Getenv("REDIS_PASSWORD")) // optional, can be empty

	cfg.RedisDB, err = getInt("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}

	// ✅ Token version cache TTL (optional)
	cfg.TokenVersionCacheTTL, err = getDuration("TOKEN_VERSION_CACHE_TTL", 10*time.Minute)
	if err != nil {
		return nil, err
	}

	cfg.RabbitURL = strings.TrimSpace(os.Getenv("RABBIT_URL"))
	if cfg.RabbitURL == "" {
		return nil, fmt.Errorf("missing required env var: RABBIT_URL")
	}

	// Timeouts (optional)
	cfg.HTTPReadTimeout, err = getDuration("HTTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}
	cfg.HTTPWriteTimeout, err = getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	cfg.HTTPIdleTimeout, err = getDuration("HTTP_IDLE_TIMEOUT", time.Minute)
	if err != nil {
		return nil, err
	}

	// Debug flags
	cfg.DBDebug = parseBool(getEnv("DB_DEBUG", "false"))

	// OAuth configuration (optional - only required if using OAuth)
	cfg.GoogleClientID = getEnv("GOOGLE_CLIENT_ID", "")
	cfg.GoogleClientSecret = getEnv("GOOGLE_CLIENT_SECRET", "")
	cfg.OAuthStateTTL, err = getDuration("OAUTH_STATE_TTL", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	cfg.OAuthCallbackURL = getEnv("OAUTH_CALLBACK_URL", "http://localhost:8080/auth/v1/oauth/google/callback")
	cfg.FrontendOrigin = getEnv("FRONTEND_ORIGIN", "http://localhost:3000")
	cfg.AllowedRedirects = parseStringList(getEnv("ALLOWED_REDIRECTS", "/,/events,/profile,/settings"))

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

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %q: %w", key, v, err)
	}
	return d, nil
}

func getInt(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid int for %s: %q: %w", key, v, err)
	}
	return n, nil
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func validatePostgresDSN(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return err
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("scheme must be postgres/postgresql, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	// must have db name path like /app
	if strings.Trim(u.Path, "/") == "" {
		return fmt.Errorf("missing database name in path, expected /<db>")
	}
	return nil
}

func parseStringList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
