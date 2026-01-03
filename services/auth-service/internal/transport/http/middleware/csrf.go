package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// CSRFProtection validates Origin/Referer headers for cookie-based endpoints.
// This protects against CSRF attacks by ensuring requests come from allowed origins.
//
// Apply to endpoints that:
// 1. Use cookies for authentication (refresh, logout)
// 2. Perform state-changing operations
func CSRFProtection(allowedOrigins []string) func(http.Handler) http.Handler {
	// Build a set of allowed hosts for fast lookup
	allowedHosts := make(map[string]struct{})
	for _, origin := range allowedOrigins {
		if u, err := url.Parse(origin); err == nil {
			allowedHosts[strings.ToLower(u.Host)] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only validate for state-changing methods
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Get Origin header first (preferred)
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Fallback to Referer
				origin = r.Header.Get("Referer")
			}

			// If no origin header, reject the request
			if origin == "" {
				http.Error(w, `{"error":"missing_origin","message":"Origin or Referer header required"}`, http.StatusForbidden)
				return
			}

			// Parse the origin and validate
			u, err := url.Parse(origin)
			if err != nil {
				http.Error(w, `{"error":"invalid_origin","message":"Invalid Origin header"}`, http.StatusForbidden)
				return
			}

			host := strings.ToLower(u.Host)
			if _, ok := allowedHosts[host]; !ok {
				http.Error(w, `{"error":"csrf_rejected","message":"Cross-origin request not allowed"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// DefaultAllowedOrigins returns sensible defaults for local development
func DefaultAllowedOrigins() []string {
	return []string{
		"http://localhost:3000", // Frontend dev server
		"http://localhost:8080", // BFF
		"http://127.0.0.1:3000",
		"http://127.0.0.1:8080",
	}
}
