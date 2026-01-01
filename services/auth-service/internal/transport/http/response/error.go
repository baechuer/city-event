package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/logger"
)

type ErrorBody struct {
	Error ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Meta      map[string]string `json:"meta,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// WriteError converts a domain error into a consistent JSON HTTP error response.
// Non-domain errors are treated as internal errors (500) without leaking details.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "internal error"
	var meta map[string]string

	var de *domain.Error
	if errors.As(err, &de) {
		status = statusFromKind(de.Kind)
		code = de.Code
		message = de.Message
		meta = de.Meta
	}

	reqID := RequestIDFromContext(r)

	// LOGGING: Log failure
	logEvent := logger.WithCtx(r.Context()).Info()
	if status >= 500 {
		logEvent = logger.WithCtx(r.Context()).Error().Err(err)
	} else if status >= 400 {
		logEvent = logger.WithCtx(r.Context()).Warn().Err(err)
	}

	logEvent.
		Str("code", code).
		Int("status", status).
		Str("path", r.URL.Path).
		Msg("http_response_error")

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(ErrorBody{
		Error: ErrorPayload{
			Code:      code,
			Message:   message,
			Meta:      meta,
			RequestID: reqID,
		},
	})
}

// statusFromKind maps domain error kinds to HTTP status codes.
func statusFromKind(kind domain.ErrKind) int {
	switch kind {
	case domain.KindValidation:
		return http.StatusBadRequest
	case domain.KindAuth:
		return http.StatusUnauthorized
	case domain.KindForbidden:
		return http.StatusForbidden
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict:
		return http.StatusConflict
	case domain.KindRateLimited:
		return http.StatusTooManyRequests
	case domain.KindInfrastructure:
		return http.StatusServiceUnavailable
	case domain.KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
