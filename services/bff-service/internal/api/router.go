package api

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/config"
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

	// 2. Health check (BFF itself)
	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 3. Auth Service Proxy
	// Map /api/auth -> /auth/v1
	authProxy, err := proxy.New(cfg.AuthServiceURL, "/api/auth", "/auth/v1")
	if err != nil {
		log.Fatalf("Invalid Auth URL: %v", err)
	}
	r.Mount("/api/auth", authProxy)

	// 4. Event Service Proxy
	// Map /api/events -> /event/v1/events
	eventProxy, err := proxy.New(cfg.EventServiceURL, "/api/events", "/event/v1/events")
	if err != nil {
		log.Fatalf("Invalid Event URL: %v", err)
	}
	r.Mount("/api/events", eventProxy)

	// 5. Feed Proxy (Strict Contract)
	// Map /api/feed -> /event/v1/events
	feedProxy, err := proxy.New(cfg.EventServiceURL, "/api/feed", "/event/v1/events")
	if err != nil {
		log.Fatalf("Invalid Feed URL: %v", err)
	}
	r.Mount("/api/feed", feedProxy)

	log.Printf("Routes Mounted:")
	log.Printf("  /api/auth   -> %s/auth/v1", cfg.AuthServiceURL)
	log.Printf("  /api/events -> %s/event/v1/events", cfg.EventServiceURL)

	return r
}
