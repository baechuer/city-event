package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
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

	r.Get("/healthz", z.Healthz)

	r.Route("/event/v1", func(r chi.Router) {
		r.Get("/events", h.ListPublic)
		r.Get("/events/{event_id}", h.GetPublic)

		r.Group(func(r chi.Router) {
			r.Use(auth.Require)
			r.Post("/events", h.Create)
			r.Patch("/events/{event_id}", h.Update)
			r.Post("/events/{event_id}/publish", h.Publish)
			r.Post("/events/{event_id}/cancel", h.Cancel)
			r.Get("/organizer/events", h.ListMine)
		})
	})

	return r
}
