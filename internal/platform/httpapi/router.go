package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg config.Config, logger *slog.Logger) http.Handler {
	return NewBaseRouter(cfg, logger)
}

func NewBaseRouter(cfg config.Config, logger *slog.Logger) chi.Router {
	return newBaseRouterWithRateLimitStore(cfg, logger, newRateLimitStore(cfg))
}

func newBaseRouterWithRateLimitStore(cfg config.Config, logger *slog.Logger, store rateLimitStore) chi.Router {
	if logger == nil {
		logger = slog.Default()
	}
	_ = logger

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(localCORSMiddleware(cfg))
	r.Use(observabilityMiddleware(cfg, logger))
	r.Use(rateLimitMiddlewareWithStore(cfg, store))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		health.WriteJSON(w, http.StatusOK, map[string]string{
			"service":     cfg.Service.Name,
			"environment": cfg.Environment,
			"status":      "running",
		})
	})

	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
		health.WriteJSON(w, http.StatusOK, health.Live(cfg.Service.Name, cfg.Environment))
	})

	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		health.WriteJSON(w, http.StatusOK, health.Ready(cfg.Service.Name, cfg.Environment))
	})

	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !authorizedForMetrics(cfg, r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="cityevents-metrics"`)
			http.Error(w, "metrics authentication required", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(observability.MetricsText()))
	})

	return r
}

func authorizedForMetrics(cfg config.Config, r *http.Request) bool {
	expected := strings.TrimSpace(cfg.MetricsBearerToken)
	if expected == "" {
		return true
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(expected)) == 1
}

func localCORSMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		allowedOrigins := cfg.AllowedOrigins
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(allowedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token, Idempotency-Key, X-Correlation-ID, traceparent, tracestate")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
