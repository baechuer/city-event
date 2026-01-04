package middleware

import "net/http"

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP for API: relaxed policy to allow OAuth callbacks and dev tools
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self' http://localhost:* ws://localhost:*; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")

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

		// Prevent window.opener access from cross-origin windows, but allow for popups (needed for OAuth)
		w.Header().Set("Cross-Origin-Opener-Policy", "unsafe-none")

		// Disable all browser features for API endpoints
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), bluetooth=()")

		next.ServeHTTP(w, r)
	})
}
