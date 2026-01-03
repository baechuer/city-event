package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/middleware"
	"github.com/google/uuid"
)

// TrackRepo defines the persistence interface for track events
type TrackRepo interface {
	InsertOutbox(ctx context.Context, e domain.TrackEvent) error
}

// TrackHandler handles /track requests
type TrackHandler struct {
	repo TrackRepo
}

func NewTrackHandler(repo TrackRepo) *TrackHandler {
	return &TrackHandler{repo: repo}
}

// Track handles POST /track - async event tracking
func (h *TrackHandler) Track(w http.ResponseWriter, r *http.Request) {
	var req domain.TrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate event_type
	if req.EventType != "impression" && req.EventType != "view" {
		http.Error(w, "invalid event_type", http.StatusBadRequest)
		return
	}

	// Validate event_id
	eventID, err := uuid.Parse(req.EventID)
	if err != nil {
		http.Error(w, "invalid event_id", http.StatusBadRequest)
		return
	}

	// Build actor_key
	var actorKey string
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		actorKey = "u:" + userID
	} else if anonID, ok := middleware.AnonIDFromContext(r.Context()); ok {
		actorKey = "a:" + anonID
	} else {
		http.Error(w, "no actor identity", http.StatusBadRequest)
		return
	}

	// Build event
	now := time.Now().UTC()
	event := domain.TrackEvent{
		ActorKey:   actorKey,
		EventType:  req.EventType,
		EventID:    eventID,
		FeedType:   req.FeedType,
		Position:   req.Position,
		RequestID:  req.RequestID,
		BucketDate: now.Truncate(24 * time.Hour), // UTC day
		OccurredAt: now,
	}

	// Insert to outbox (best-effort, don't block on errors)
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
	defer cancel()

	if err := h.repo.InsertOutbox(ctx, event); err != nil {
		// Log but don't fail the request
		// TODO: add metrics for outbox insert failures
	}

	w.WriteHeader(http.StatusAccepted)
}
