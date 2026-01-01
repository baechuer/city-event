package context

import (
	"context"
)

type key int

const (
	requestIDKey key = iota
	userIDKey
)

func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, requestIDKey, reqID)
}

func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(requestIDKey).(string); ok {
		return val
	}
	return ""
}
