package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg config.Config, logger *slog.Logger) http.Handler {
	return NewBaseRouter(cfg, logger)
}

func NewBaseRouter(cfg config.Config, logger *slog.Logger) chi.Router {
	if logger == nil {
		logger = slog.Default()
	}
	_ = logger

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(localCORSMiddleware)

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

	return r
}

func localCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-User-ID, Idempotency-Key")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
