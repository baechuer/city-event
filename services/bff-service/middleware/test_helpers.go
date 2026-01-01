package middleware

import "context"

// SetRequestIDForTest is a helper to inject a request ID into the context for testing.
// It bypasses the HTTP middleware verification.
func SetRequestIDForTest(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, id)
}
