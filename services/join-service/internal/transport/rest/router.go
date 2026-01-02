package rest

import (
	"net/http"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type RouterDeps struct {
	Cache     domain.CacheRepository
	Handler   *Handler
	Verifier  security.AccessTokenVerifier
	JWTIssuer string
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
	r.Use(HTTPLogger)

	// Panic recovery
	r.Use(middleware.Recoverer)

	// Cross-cutting
	r.Use(RateLimitMiddleware(d.Cache))
	r.Use(SecurityHeaders)

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
