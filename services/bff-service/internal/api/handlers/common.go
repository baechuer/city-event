package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/domain"
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
