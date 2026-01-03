package router

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/handlers"
	authmw "github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/middleware"
)

func New(
	h *handlers.EventsHandler,
	auth *authmw.AuthMiddleware,
	z *handlers.HealthHandler,
	rdb *redis.Client,
	cfg *config.Config,
) http.Handler {
	r := chi.NewRouter()

	r.Use(authmw.RequestID)
	r.Use(authmw.Metrics) // Prometheus metrics
	r.Use(authmw.SecurityHeaders)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(authmw.AccessLog)

	if cfg.RLEnabled {
		if rdb == nil {
			r.Use(httprate.LimitByIP(cfg.RLLimit, cfg.RLWindow))
		} else {
			r.Use(httprate.Limit(
				cfg.RLLimit,
				cfg.RLWindow,
				httprate.WithKeyFuncs(httprate.KeyByIP),
			))
		}
	}

	// Operational endpoints
	r.Get("/healthz", z.Healthz)
	r.Get("/readyz", readyzHandler(rdb, cfg))
	r.Handle("/metrics", promhttp.Handler())

	// Also expose at /event/v1/health for BFF readiness checks
	r.Get("/event/v1/health", z.Healthz)

	r.Route("/event/v1", func(r chi.Router) {
		r.Get("/events", h.ListPublic)
		r.Post("/events/batch", h.GetBatch) // Batch lookup for N+1 fix
		r.Get("/events/{event_id}", h.GetPublic)
		r.Get("/meta/cities", h.GetCitySuggestions)

		r.Group(func(r chi.Router) {
			r.Use(auth.Require)
			r.Post("/events", h.Create)
			r.Patch("/events/{event_id}", h.Update)
			r.Post("/events/{event_id}/publish", h.Publish)
			r.Post("/events/{event_id}/unpublish", h.Unpublish)
			r.Post("/events/{event_id}/cancel", h.Cancel)
			r.Get("/organizer/events", h.ListMine)
			r.Get("/organizer/events/{event_id}", h.GetMine)
		})
	})

	return r
}

// readyzHandler checks DB and Redis connectivity
func readyzHandler(rdb *redis.Client, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		checks := make(map[string]string)
		allHealthy := true

		// Check Redis if configured
		if rdb != nil {
			if err := rdb.Ping(ctx).Err(); err != nil {
				checks["redis"] = "unhealthy: " + err.Error()
				allHealthy = false
			} else {
				checks["redis"] = "healthy"
			}
		} else {
			checks["redis"] = "not_configured"
		}

		// Note: DB check would require injecting db pool here
		// For now, we just check Redis
		checks["status"] = "ready"
		if !allHealthy {
			checks["status"] = "not_ready"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(checks)
	}
}
