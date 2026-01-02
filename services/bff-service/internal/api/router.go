package api

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/api/handlers"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/downstream"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/logger"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/proxy"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
)

func NewRouter(cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// 2. Middleware
	// Replace default chi Logger with our structured logger
	r.Use(middleware.RequestLogger(logger.Log))
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.SecurityHeaders)

	// 6. Business Handlers (Aggregation & Policy)
	eventClient := downstream.NewEventClient(cfg.EventServiceURL)
	joinClient := downstream.NewJoinClient(cfg.JoinServiceURL)
	authClient := downstream.NewAuthClient(cfg.AuthServiceURL)
	eventHandler := handlers.NewEventHandler(eventClient, joinClient, authClient)

	// 2. Health check and Proxies
	r.Route("/api", func(r chi.Router) {
		r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		// Auth Service Proxy
		authProxy, err := proxy.New(cfg.AuthServiceURL, "/api/auth", "/auth/v1")
		if err != nil {
			log.Fatalf("Invalid Auth URL: %v", err)
		}
		r.Mount("/auth", authProxy)

		// Business Handlers (Authenticated)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(cfg.JWTSecret))

			r.Get("/feed", eventHandler.ListFeed)
			r.Get("/me/joins", eventHandler.ListMyJoins)
			r.Get("/events", eventHandler.ListEvents)
			r.Post("/events", eventHandler.CreateEvent)
			r.Get("/events/{id}/view", eventHandler.GetEventView)
			r.Post("/events/{id}/publish", eventHandler.PublishEvent)
			r.Post("/events/{id}/join", eventHandler.JoinEvent)
			r.Post("/events/{id}/cancel", eventHandler.CancelJoin)
		})
	})

	log.Printf("Routes Mounted:")
	log.Printf("  /api/auth   -> %s/auth/v1", cfg.AuthServiceURL)
	log.Printf("  /api/events -> %s/event/v1/events", cfg.EventServiceURL)

	return r
}
