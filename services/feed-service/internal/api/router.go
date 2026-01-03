package api

import (
	"net/http"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/api/handlers"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/middleware"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg *config.Config, trackHandler *handlers.TrackHandler, feedHandler *handlers.FeedHandler) http.Handler {
	r := chi.NewRouter()

	// Base middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	// Anon ID middleware (HMAC-signed cookie)
	secureCookie := cfg.Env != "dev"
	r.Use(middleware.AnonID(cfg.AnonCookieSecret, cfg.AnonCookieTTL, secureCookie))

	// Rate limiting
	actorLimiter := middleware.NewRateLimiter(cfg.RateLimitPerActor, time.Minute)
	ipLimiter := middleware.NewRateLimiter(cfg.RateLimitPerIP, time.Minute)

	// Routes
	r.Route("/api/feed", func(r chi.Router) {
		// Public routes
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		// Track endpoint with rate limiting
		r.With(middleware.RateLimit(actorLimiter, ipLimiter)).Post("/track", trackHandler.Track)

		// Feed endpoint
		r.Get("/", feedHandler.GetFeed)
	})

	return r
}
