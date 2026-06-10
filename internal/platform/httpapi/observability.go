package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/go-chi/chi/v5/middleware"
)

const CorrelationIDHeader = observability.CorrelationIDHeader

func observabilityMiddleware(cfg config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := strings.TrimSpace(r.Header.Get(CorrelationIDHeader))
			if correlationID == "" {
				correlationID = observability.NewCorrelationID()
			}
			w.Header().Set(CorrelationIDHeader, correlationID)
			ctx := observability.ContextWithCorrelationID(r.Context(), correlationID)
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r.WithContext(ctx))
			duration := time.Since(start)
			observability.RecordHTTPRequest(cfg.Service.Name, rec.status)
			logger.Info("http request completed",
				slog.String("service", cfg.Service.Name),
				slog.String("request_id", middleware.GetReqID(ctx)),
				slog.String("correlation_id", correlationID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", duration.Milliseconds()),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}
