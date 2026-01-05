package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RouterDeps struct {
	Cache     domain.CacheRepository
	Handler   *Handler
	Verifier  security.AccessTokenVerifier
	JWTIssuer string
	RLLimit   int
	RLWindow  time.Duration
}

func NewRouter(d RouterDeps) http.Handler {
	if d.Cache == nil {
		panic("rest.NewRouter: nil cache")
	}
	if d.Handler == nil {
		panic("rest.NewRouter: nil handler")
	}
	if d.Verifier == nil {
		panic("rest.NewRouter: nil verifier")
	}

	r := chi.NewRouter()

	// Request ID + structured access log
	r.Use(RequestID)
	r.Use(MetricsMiddleware) // Prometheus metrics
	r.Use(HTTPLogger)

	// Panic recovery
	r.Use(middleware.Recoverer)

	// Cross-cutting
	r.Use(RateLimitMiddleware(d.Cache, d.RLLimit, d.RLWindow))
	r.Use(SecurityHeaders)

	// Operational endpoints (outside /api for K8s probes)
	r.Get("/healthz", healthzHandler)
	r.Get("/readyz", readyzHandler(d.Cache))
	r.Handle("/metrics", promhttp.Handler())

	// Also expose at /join/v1/health for BFF readiness checks
	r.Get("/join/v1/health", healthzHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(AuthMiddleware(d.Verifier, AuthOptions{ExpectedIssuer: d.JWTIssuer}))

		// existing
		r.Post("/join", d.Handler.Join)
		r.Delete("/join/{eventID}", d.Handler.Cancel)

		// reads
		r.Get("/me/joins", d.Handler.MeJoins)
		r.Get("/events/{eventID}/participation", d.Handler.GetMyParticipation)

		r.Get("/events/{eventID}/participants", d.Handler.Participants)
		r.Get("/events/{eventID}/waitlist", d.Handler.Waitlist)
		r.Get("/events/{eventID}/stats", d.Handler.Stats)

		// moderation
		r.Delete("/events/{eventID}/participants/{userID}", d.Handler.Kick)
		r.Post("/events/{eventID}/bans", d.Handler.Ban)
		r.Delete("/events/{eventID}/bans/{userID}", d.Handler.Unban)
	})

	return r
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func readyzHandler(cache domain.CacheRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		checks := make(map[string]string)
		allHealthy := true

		// Check Redis via cache
		if cache != nil {
			if err := cache.Ping(ctx); err != nil {
				checks["redis"] = "unhealthy: " + err.Error()
				allHealthy = false
			} else {
				checks["redis"] = "healthy"
			}
		} else {
			checks["redis"] = "not_configured"
		}

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
