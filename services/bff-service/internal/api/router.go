package api

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/api/handlers"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/downstream"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/logger"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/proxy"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
)

func NewRouter(cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// 1. Request ID (must be first for tracing)
	r.Use(middleware.RequestID)

	// 2. Tracing (OpenTelemetry) - if enabled
	if cfg.TracingEnabled {
		r.Use(middleware.Tracing("bff-service"))
		log.Println("Tracing: OpenTelemetry enabled")
	}

	// 3. Metrics (Prometheus RED metrics)
	r.Use(middleware.Metrics)

	// 3. CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Idempotency-Key", "X-Request-Id"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 4. Rate Limit (distributed if Redis is available, otherwise in-memory fallback)
	if cfg.RLEnabled {
		if cfg.RedisAddr != "" {
			// Use Redis-backed distributed rate limiter
			rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
			rateLimiter := middleware.NewRedisRateLimiter(rdb)
			r.Use(rateLimiter.Middleware(middleware.RateLimitConfig{
				Limit:  cfg.RLLimit,
				Window: cfg.RLWindow,
				KeyFn:  middleware.KeyByUser, // User-based if authenticated, IP fallback
			}))
			log.Println("Rate limiting: Redis-backed (distributed)")
		} else {
			// Fallback to in-memory (single-node only)
			r.Use(httprate.Limit(
				cfg.RLLimit,
				cfg.RLWindow,
				httprate.WithKeyFuncs(httprate.KeyByIP),
			))
			log.Println("Rate limiting: In-memory (single-node only)")
		}
	}

	r.Use(middleware.Auth(cfg.JWTSecret))
	r.Use(middleware.RequestLogger(logger.Log))
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.SecurityHeaders)

	// 5. Downstream Clients
	eventClient := downstream.NewEventClient(cfg.EventServiceURL)
	joinClient := downstream.NewJoinClient(cfg.JoinServiceURL)
	authClient := downstream.NewAuthClient(cfg.AuthServiceURL, cfg.InternalSecretKey)
	eventHandler := handlers.NewEventHandler(eventClient, joinClient, authClient)

	// 6. Readiness checks (for downstream services)
	readinessHandler := handlers.NewReadinessHandler(
		handlers.NewHTTPReadinessChecker("auth-service", cfg.AuthServiceURL+"/auth/v1/health"),
		handlers.NewHTTPReadinessChecker("event-service", cfg.EventServiceURL+"/event/v1/health"),
		handlers.NewHTTPReadinessChecker("join-service", cfg.JoinServiceURL+"/join/v1/health"),
	)

	// 7. Operational endpoints (outside /api for Kubernetes probes)
	r.Get("/healthz", readinessHandler.Healthz)
	r.Get("/readyz", readinessHandler.Readyz)
	r.Handle("/metrics", promhttp.Handler())

	// 8. API routes
	r.Route("/api", func(r chi.Router) {
		// Legacy healthz (keep for backwards compatibility)
		r.Get("/healthz", readinessHandler.Healthz)

		// Auth Service Proxy
		authProxy, err := proxy.New(cfg.AuthServiceURL, "/api/auth", "/auth/v1")
		if err != nil {
			log.Fatalf("Invalid Auth URL: %v", err)
		}
		r.Mount("/auth", authProxy)

		// Feed Service Proxy (for recommendation feed)
		feedProxy, err := proxy.New(cfg.FeedServiceURL, "/api/feed/recommended", "/api/feed")
		if err != nil {
			log.Fatalf("Invalid Feed URL: %v", err)
		}
		r.Mount("/feed/recommended", feedProxy)

		// Public Routes (No Auth Required, but context is populated if token is present)
		r.Get("/feed", eventHandler.ListFeed) // Legacy fallback
		r.Get("/events/{id}/view", eventHandler.GetEventView)
		r.Get("/meta/cities", eventHandler.GetCitySuggestions)
		r.Get("/media/{id}/status", handlers.NewMediaHandler(cfg.MediaServiceURL).GetStatus)

		// Business Handlers (Authenticated)
		r.Group(func(r chi.Router) {
			r.Get("/me/joins", eventHandler.ListMyJoins)
			r.Get("/me/events", eventHandler.ListCreatedEvents)
			r.Get("/events", eventHandler.ListEvents)
			r.Post("/events", eventHandler.CreateEvent)
			r.Patch("/events/{id}", eventHandler.UpdateEvent)
			r.Post("/events/{id}/publish", eventHandler.PublishEvent)
			r.Post("/events/{id}/cancel-event", eventHandler.CancelEvent)
			r.Post("/events/{id}/unpublish", eventHandler.UnpublishEvent)
			r.Post("/events/{id}/join", eventHandler.JoinEvent)
			r.Post("/events/{id}/cancel", eventHandler.CancelJoin)

			// Media Upload Routes
			mediaHandler := handlers.NewMediaHandler(cfg.MediaServiceURL)
			r.Post("/media/request-upload", mediaHandler.RequestUpload)
			r.Post("/media/complete", mediaHandler.CompleteUpload)
		})

		// Admin/Moderator Routes
		r.Group(func(r chi.Router) {
			r.Use(RequireRole("admin", "moderator"))
			r.Post("/admin/events/{id}/cancel", eventHandler.AdminCancelEvent)
			r.Post("/admin/events/{id}/unpublish", eventHandler.AdminUnpublishEvent)
		})
	})

	log.Printf("Routes Mounted:")
	log.Printf("  /healthz    -> Liveness probe")
	log.Printf("  /readyz     -> Readiness probe (checks downstream)")
	log.Printf("  /metrics    -> Prometheus metrics")
	log.Printf("  /api/auth   -> %s/auth/v1", cfg.AuthServiceURL)
	log.Printf("  /api/events -> %s/event/v1/events", cfg.EventServiceURL)

	return r
}

func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := middleware.GetUserRole(r.Context())
			for _, allowed := range allowedRoles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
		})
	}
}
