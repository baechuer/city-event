package middleware

import (
	"net/http"

	"github.com/google/uuid"

	appCtx "github.com/baechuer/real-time-ressys/services/event-service/internal/pkg/context"
)

const HeaderXRequestID = "X-Request-Id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(HeaderXRequestID)

		if reqID == "" {
			reqID = uuid.NewString()
		}

		w.Header().Set(HeaderXRequestID, reqID)

		ctx := appCtx.WithRequestID(r.Context(), reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
