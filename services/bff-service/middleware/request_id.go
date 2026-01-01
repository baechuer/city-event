package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const HeaderXRequestID = "X-Request-Id"

type ctxKeyRequestID struct{}

// RequestID Middleware for BFF
// Note: BFF usually generates the ID if missing, AND propagates it to downstream.
// Downstream propagation is handled by the HTTP Client logic, but this middleware establishes the ID in context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(HeaderXRequestID)

		if reqID == "" {
			reqID = uuid.NewString()
		}

		w.Header().Set(HeaderXRequestID, reqID)

		// Create a context with the Request ID
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Helper to get ID from context (for logging or downstream calls)
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
		return reqID
	}
	return ""
}
