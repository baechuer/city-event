package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/downstream"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
)

func sendError(w http.ResponseWriter, r *http.Request, code string, message string, status int) {
	resp := domain.APIError{}
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.RequestID = middleware.GetRequestID(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func handleDownstreamError(w http.ResponseWriter, r *http.Request, err error, defaultMsg string) {
	var se *downstream.StatusError
	if errors.As(err, &se) {
		sendError(w, r, se.Code, se.Message, se.StatusCode)
		return
	}
	sendError(w, r, "internal_error", defaultMsg, http.StatusBadGateway)
}
