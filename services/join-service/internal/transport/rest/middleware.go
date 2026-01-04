package rest

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/security"
	"github.com/google/uuid"
)

type AuthOptions struct {
	// If set (non-empty), enforce exact issuer match.
	ExpectedIssuer string
}

func AuthMiddleware(verifier security.AccessTokenVerifier, opt AuthOptions) func(next http.Handler) http.Handler {
	if verifier == nil {
		panic("AuthMiddleware: nil verifier")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := strings.TrimSpace(r.Header.Get("Authorization"))
			if h == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			raw := strings.TrimSpace(parts[1])
			if raw == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			claims, err := verifier.VerifyAccessToken(raw)
			if err != nil {
				// You can distinguish expired vs invalid if you want different messages;
				// but status stays 401 either way.
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if opt.ExpectedIssuer != "" && claims.Issuer != opt.ExpectedIssuer {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if strings.TrimSpace(claims.UserID) == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			uid, err := uuid.Parse(claims.UserID)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := withAuth(r.Context(), AuthContext{
				UserID: uid,
				Role:   strings.TrimSpace(claims.Role),
				Ver:    claims.Ver,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RateLimitMiddleware(cache domain.CacheRepository) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			allowed, _ := cache.AllowRequest(r.Context(), ip, 100, time.Minute)
			if !allowed {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP keeps it simple: RemoteAddr host part.
// If you are behind a trusted reverse proxy, you may choose to trust X-Forwarded-For,
// but doing so blindly is a spoofing risk.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP for API: restrictive policy suitable for JSON-only endpoints
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")

		// HSTS: Enforce HTTPS for 1 year, include subdomains
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking (redundant with CSP frame-ancestors, but belt-and-suspenders)
		w.Header().Set("X-Frame-Options", "DENY")

		// XSS protection (legacy but harmless)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Don't leak referrer to external sites
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Prevent cross-origin resource embedding
		w.Header().Set("Cross-Origin-Resource-Policy", "same-site")

		// Prevent window.opener access from cross-origin windows
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

		// Disable all browser features for API endpoints
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), bluetooth=()")

		next.ServeHTTP(w, r)
	})
}
