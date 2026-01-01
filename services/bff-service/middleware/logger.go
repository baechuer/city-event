package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// RequestLogger returns a middleware that logs HTTP requests
func RequestLogger(l zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Log Request Start? No, usually noisy. Log completion is enough.

			next.ServeHTTP(ww, r)

			// Log Request End
			latency := time.Since(start)

			// Context has RequestID if RequestID middleware ran before this
			reqID := GetRequestID(r.Context())

			event := l.Info()
			if ww.Status() >= 500 {
				event = l.Error()
			} else if ww.Status() >= 400 {
				event = l.Warn()
			}

			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("latency", latency).
				Str("request_id", reqID).
				Str("ip", r.RemoteAddr).
				Msg("http_request")
		})
	}
}
