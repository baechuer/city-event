package rest

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	appCtx "github.com/baechuer/real-time-ressys/services/join-service/internal/pkg/context"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/service"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/transport/rest/response"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Handler struct {
	svc *service.JoinService
}

func NewHandler(svc *service.JoinService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID string `json:"event_id"`
	}
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid body", nil)
		return
	}

	eventID, err := uuid.Parse(req.EventID)
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid event_id", map[string]string{
			"event_id": "must be a valid uuid",
		})
		return
	}

	traceID := appCtx.GetRequestID(r.Context())
	if traceID == "" {
		traceID = "no-request-id"
	}

	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	// X-Idempotency-Key is REQUIRED for write operations
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = r.Header.Get("Idempotency-Key") // legacy fallback
	}
	if idempotencyKey == "" {
		fail(w, r, http.StatusBadRequest, "idempotency_key.required", "X-Idempotency-Key header is required for this operation", nil)
		return
	}

	status, err := h.svc.Join(r.Context(), traceID, idempotencyKey, eventID, auth.UserID)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	response.Data(w, http.StatusOK, map[string]string{
		"status": status, // "active" | "waitlisted"
	})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", map[string]string{
			"event_id": "must be a valid uuid",
		})
		return
	}

	traceID := appCtx.GetRequestID(r.Context())
	if traceID == "" {
		traceID = "no-request-id"
	}

	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	// X-Idempotency-Key is REQUIRED for write operations
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = r.Header.Get("Idempotency-Key") // legacy fallback
	}
	if idempotencyKey == "" {
		fail(w, r, http.StatusBadRequest, "idempotency_key.required", "X-Idempotency-Key header is required for this operation", nil)
		return
	}

	if err := h.svc.Cancel(r.Context(), traceID, idempotencyKey, eventID, auth.UserID); err != nil {
		handleErr(w, r, err)
		return
	}

	response.Data(w, http.StatusOK, map[string]string{
		"msg": "canceled",
	})
}

func handleErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrEventFull):
		fail(w, r, http.StatusConflict, "event.full", err.Error(), nil)
		return
	case errors.Is(err, domain.ErrIdempotencyKeyMismatch):
		fail(w, r, http.StatusConflict, "idempotency_key_mismatch", err.Error(), nil)
		return
	case errors.Is(err, domain.ErrAlreadyJoined):
		// BFF will treat this as success (idempotent result)
		fail(w, r, http.StatusConflict, "state_already_reached", err.Error(), nil)
		return

	case errors.Is(err, domain.ErrEventClosed):
		// 410 is semantically accurate; if you prefer 409, switch it.
		fail(w, r, http.StatusGone, "event.closed", err.Error(), nil)
		return

	case errors.Is(err, domain.ErrEventNotFound):
		fail(w, r, http.StatusNotFound, "event.not_found", err.Error(), nil)
		return
	case errors.Is(err, domain.ErrBanned):
		fail(w, r, http.StatusForbidden, "join.banned", err.Error(), nil)
		return
	case errors.Is(err, domain.ErrForbidden):
		fail(w, r, http.StatusForbidden, "auth.forbidden", err.Error(), nil)
		return
	case errors.Is(err, domain.ErrNotJoined):
		fail(w, r, http.StatusNotFound, "join.not_found", err.Error(), nil)
		return
	case errors.Is(err, domain.ErrEventNotKnown) || errors.Is(err, domain.ErrEventNotFound):
		fail(w, r, http.StatusNotFound, "event.not_found", err.Error(), nil)
		return

	default:
		// Do not leak internal details by default. If you want raw err in dev, gate by APP_ENV.
		fail(w, r, http.StatusInternalServerError, "internal", "internal error", nil)
		return
	}
}

func fail(w http.ResponseWriter, r *http.Request, status int, code, message string, meta map[string]string) {
	reqID := appCtx.GetRequestID(r.Context())
	if reqID == "" {
		reqID = "no-request-id"
	}
	response.Fail(w, status, code, message, meta, reqID)
}
func (h *Handler) MeJoins(w http.ResponseWriter, r *http.Request) {
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	cur, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid cursor", nil)
		return
	}

	// status=active,waitlisted,...
	var statuses []domain.JoinStatus
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		for _, p := range strings.Split(s, ",") {
			v := domain.JoinStatus(strings.TrimSpace(p))
			if v != "" {
				statuses = append(statuses, v)
			}
		}
	}

	var from *time.Time
	if s := strings.TrimSpace(r.URL.Query().Get("from")); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			fail(w, r, http.StatusBadRequest, "request.invalid", "invalid from", nil)
			return
		}
		tt := t.UTC()
		from = &tt
	}
	var to *time.Time
	if s := strings.TrimSpace(r.URL.Query().Get("to")); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			fail(w, r, http.StatusBadRequest, "request.invalid", "invalid to", nil)
			return
		}
		tt := t.UTC()
		to = &tt
	}

	items, next, err := h.svc.ListMyJoins(r.Context(), auth.UserID, statuses, from, to, limit, cur)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	response.Data(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": encodeCursor(next),
	})
}

func (h *Handler) Participants(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	cur, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid cursor", nil)
		return
	}

	items, next, err := h.svc.ListParticipants(r.Context(), eventID, auth.UserID, auth.Role, limit, cur)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	response.Data(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": encodeCursor(next),
	})
}

func (h *Handler) Waitlist(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	cur, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid cursor", nil)
		return
	}

	items, next, err := h.svc.ListWaitlist(r.Context(), eventID, auth.UserID, auth.Role, limit, cur)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	response.Data(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": encodeCursor(next),
	})
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	s, err := h.svc.GetStats(r.Context(), eventID, auth.UserID, auth.Role)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	response.Data(w, http.StatusOK, s)
}

func (h *Handler) Kick(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid userID", nil)
		return
	}
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	traceID := appCtx.GetRequestID(r.Context())
	if traceID == "" {
		traceID = "no-request-id"
	}

	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if err := h.svc.Kick(r.Context(), traceID, eventID, targetUserID, auth.UserID, auth.Role, reason); err != nil {
		handleErr(w, r, err)
		return
	}
	response.Data(w, http.StatusOK, map[string]any{"msg": "kicked"})
}

func (h *Handler) Ban(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	var req struct {
		UserID    string  `json:"user_id"`
		Reason    string  `json:"reason"`
		ExpiresAt *string `json:"expires_at"` // RFC3339 optional
	}
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid body", nil)
		return
	}
	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid user_id", nil)
		return
	}

	var exp *time.Time
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt))
		if err != nil {
			fail(w, r, http.StatusBadRequest, "request.invalid", "invalid expires_at", nil)
			return
		}
		tt := t.UTC()
		exp = &tt
	}

	traceID := appCtx.GetRequestID(r.Context())
	if traceID == "" {
		traceID = "no-request-id"
	}

	if err := h.svc.Ban(r.Context(), traceID, eventID, targetUserID, auth.UserID, auth.Role, req.Reason, exp); err != nil {
		handleErr(w, r, err)
		return
	}
	response.Data(w, http.StatusOK, map[string]any{"msg": "banned"})
}

func (h *Handler) Unban(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid userID", nil)
		return
	}
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	traceID := appCtx.GetRequestID(r.Context())
	if traceID == "" {
		traceID = "no-request-id"
	}
	if err := h.svc.Unban(r.Context(), traceID, eventID, targetUserID, auth.UserID, auth.Role); err != nil {
		handleErr(w, r, err)
		return
	}
	response.Data(w, http.StatusOK, map[string]string{"status": "unbanned"})
}

func (h *Handler) GetMyParticipation(w http.ResponseWriter, r *http.Request) {
	// 1. User from Context
	auth, ok := GetAuth(r.Context())
	if !ok {
		fail(w, r, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}

	// 2. EventID from path
	eventIDStr := chi.URLParam(r, "eventID")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		fail(w, r, http.StatusBadRequest, "request.invalid", "invalid eventID", nil)
		return
	}

	// 3. Service Call
	rec, err := h.svc.GetMyParticipation(r.Context(), auth.UserID, eventID)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// 4. Response
	response.Data(w, http.StatusOK, map[string]any{
		"event_id":  rec.EventID,
		"user_id":   rec.UserID,
		"status":    rec.Status,
		"joined_at": rec.CreatedAt,
	})
}

func parseLimit(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 20
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 20
	}
	if n < 1 {
		return 1
	}
	if n > 100 {
		return 100
	}
	return n
}
