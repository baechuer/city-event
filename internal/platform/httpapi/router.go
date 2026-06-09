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
	if logger == nil {
		logger = slog.Default()
	}
	_ = logger

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

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
