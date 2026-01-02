package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/downstream"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type EventClient interface {
	GetEvent(ctx context.Context, eventID uuid.UUID) (*domain.Event, error)
	ListEvents(ctx context.Context, query url.Values) (*domain.PaginatedResponse[domain.EventCard], error)
}

type JoinClient interface {
	GetParticipation(ctx context.Context, eventID, userID uuid.UUID, bearerToken string) (*domain.Participation, error)
	JoinEvent(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) (domain.ParticipationStatus, error)
	CancelJoin(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) error
}

type EventHandler struct {
	eventClient EventClient
	joinClient  JoinClient
}

func NewEventHandler(ec EventClient, jc JoinClient) *EventHandler {
	return &EventHandler{
		eventClient: ec,
		joinClient:  jc,
	}
}

type EventViewResponse struct {
	Event         *domain.Event         `json:"event"`
	Participation *domain.Participation `json:"participation"`
	Actions       domain.ActionPolicy   `json:"actions"`
	Degraded      *DegradedInfo         `json:"degraded,omitempty"`
}

type DegradedInfo struct {
	Participation string `json:"participation"`
}

func (h *EventHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()

	res, err := h.eventClient.ListEvents(ctx, r.URL.Query())
	if err != nil {
		sendError(w, r, "internal_error", "failed to fetch events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (h *EventHandler) ListFeed(w http.ResponseWriter, r *http.Request) {
	// For V0, feed uses the same logic as events list but might use different query params in frontend
	h.ListEvents(w, r)
}
func (h *EventHandler) GetEventView(w http.ResponseWriter, r *http.Request) {
	eventIDStr := chi.URLParam(r, "id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		sendError(w, r, "validation_failed", "invalid event id", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	bearerToken := middleware.GetBearerToken(r.Context())

	var wg sync.WaitGroup
	wg.Add(2)

	var (
		event    *domain.Event
		eventErr error
		part     *domain.Participation
		partErr  error
	)

	// Fetch Event
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(r.Context(), 800*time.Millisecond)
		defer cancel()
		event, eventErr = h.eventClient.GetEvent(ctx, eventID)
	}()

	// Fetch Participation
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()
		part, partErr = h.joinClient.GetParticipation(ctx, eventID, userID, bearerToken)
	}()

	wg.Wait()

	if eventErr != nil {
		if eventErr == downstream.ErrNotFound {
			sendError(w, r, "resource_not_found", "event not found", http.StatusNotFound)
			return
		}
		if eventErr == downstream.ErrTimeout {
			sendError(w, r, "upstream_timeout", "event service timeout", http.StatusGatewayTimeout)
			return
		}
		sendError(w, r, "internal_error", "failed to fetch event", http.StatusBadGateway)
		return
	}

	isDegraded := false
	var degradedInfo *DegradedInfo

	if partErr != nil {
		isDegraded = true
		msg := "unavailable"
		if partErr == downstream.ErrTimeout {
			msg = "timeout"
		}
		degradedInfo = &DegradedInfo{
			Participation: msg,
		}
		part = nil // Ensure participation is null if degraded
	}

	policy := domain.CalculateActionPolicy(event, part, userID, time.Now(), isDegraded)

	resp := EventViewResponse{
		Event:         event,
		Participation: part,
		Actions:       policy,
		Degraded:      degradedInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *EventHandler) JoinEvent(w http.ResponseWriter, r *http.Request) {
	eventIDStr := chi.URLParam(r, "id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		sendError(w, r, "validation_failed", "invalid event id", http.StatusBadRequest)
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		sendError(w, r, "validation_failed", "Idempotency-Key header is required", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == uuid.Nil {
		sendError(w, r, "unauthorized", "auth required", http.StatusUnauthorized)
		return
	}

	bearerToken := middleware.GetBearerToken(r.Context())
	requestID := middleware.GetRequestID(r.Context())

	status, err := h.joinClient.JoinEvent(r.Context(), eventID, bearerToken, idempotencyKey, requestID)
	if err != nil {
		if errors.Is(err, domain.ErrIdempotencyKeyMismatch) {
			sendError(w, r, "conflict_state", "idempotency key mismatch", http.StatusConflict)
			return
		}
		if errors.Is(err, domain.ErrAlreadyJoined) {
			rec, _ := h.joinClient.GetParticipation(r.Context(), eventID, userID, bearerToken)
			h.respondWithJoinState(w, eventID, userID, rec.Status)
			return
		}
		sendError(w, r, "internal_error", err.Error(), http.StatusBadGateway)
		return
	}

	h.respondWithJoinState(w, eventID, userID, status)
}

func (h *EventHandler) CancelJoin(w http.ResponseWriter, r *http.Request) {
	eventIDStr := chi.URLParam(r, "id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		sendError(w, r, "validation_failed", "invalid event id", http.StatusBadRequest)
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		sendError(w, r, "validation_failed", "Idempotency-Key header is required", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == uuid.Nil {
		sendError(w, r, "unauthorized", "auth required", http.StatusUnauthorized)
		return
	}

	bearerToken := middleware.GetBearerToken(r.Context())
	requestID := middleware.GetRequestID(r.Context())

	err = h.joinClient.CancelJoin(r.Context(), eventID, bearerToken, idempotencyKey, requestID)
	if err != nil {
		sendError(w, r, "internal_error", err.Error(), http.StatusBadGateway)
		return
	}

	h.respondWithJoinState(w, eventID, userID, domain.StatusCanceled)
}

func (h *EventHandler) respondWithJoinState(w http.ResponseWriter, eventID uuid.UUID, userID uuid.UUID, status domain.ParticipationStatus) {
	resp := map[string]any{
		"event_id":   eventID,
		"user_id":    userID,
		"status":     status,
		"updated_at": time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
