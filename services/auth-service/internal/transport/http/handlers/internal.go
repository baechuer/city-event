package http_handlers

import (
	"net/http"
	"strings"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/dto"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/response"
	"github.com/go-chi/chi/v5"
)

// InternalGetUser returns full user details (including Email) for internal services.
// WARNING: This endpoint exposes PII and must NOT be exposed publicly.
// It should be protected by network policies (internal only) or mTLS/API Keys.
func (h *AuthHandler) InternalGetUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if strings.TrimSpace(targetID) == "" {
		response.WriteError(w, r, domain.ErrMissingField("id"))
		return
	}

	u, err := h.svc.GetUserByID(r.Context(), targetID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	// Returns full UserView including Email
	data := dto.UserView{
		ID:            u.ID,
		Email:         u.Email,
		Role:          u.Role,
		EmailVerified: u.EmailVerified,
		Locked:        u.Locked,
		HasPassword:   u.PasswordHash != "",
	}

	response.OK(w, data)
}
