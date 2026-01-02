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
	CreateEvent(ctx context.Context, bearerToken string, body interface{}) (*domain.Event, error)
	PublishEvent(ctx context.Context, bearerToken, eventID string) (*domain.Event, error)
}

type JoinClient interface {
	GetParticipation(ctx context.Context, eventID, userID uuid.UUID, bearerToken string) (*domain.Participation, error)
	JoinEvent(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) (domain.ParticipationStatus, error)
	CancelJoin(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) error
	ListMyJoins(ctx context.Context, bearerToken string, query url.Values) (*domain.PaginatedResponse[domain.JoinRecord], error)
}

type AuthClient interface {
	GetUser(ctx context.Context, userID uuid.UUID) (*domain.User, error)
}

type EventHandler struct {
	eventClient EventClient
	joinClient  JoinClient
	authClient  AuthClient
}

func NewEventHandler(ec EventClient, jc JoinClient, ac AuthClient) *EventHandler {
	return &EventHandler{
		eventClient: ec,
		joinClient:  jc,
		authClient:  ac,
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
		handleDownstreamError(w, r, err, "failed to fetch events")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (h *EventHandler) ListFeed(w http.ResponseWriter, r *http.Request) {
	// For V0, feed uses the same logic as events list but might use different query params in frontend
	h.ListEvents(w, r)
}

func (h *EventHandler) ListMyJoins(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second) // Increased timeout for enrichment
	defer cancel()

	bearerToken := middleware.GetBearerToken(r.Context())

	// 1. Get raw join records (ID=JoinID, EventID=EventID)
	joinRes, err := h.joinClient.ListMyJoins(ctx, bearerToken, r.URL.Query())
	if err != nil {
		if errors.Is(err, downstream.ErrUnauthorized) {
			sendError(w, r, "unauthorized", "session expired", http.StatusUnauthorized)
			return
		}
		handleDownstreamError(w, r, err, "failed to fetch your activities")
		return
	}

	// 2. Extract Event IDs
	items := joinRes.Items
	if len(items) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain.PaginatedResponse[domain.EventCard]{
			Items:      []domain.EventCard{},
			NextCursor: joinRes.NextCursor,
			HasMore:    joinRes.HasMore,
		})
		return
	}

	// 3. Concurrently fetch details for each event
	var wg sync.WaitGroup
	// Semaphore to limit concurrency
	sem := make(chan struct{}, 5)

	eventMap := make(map[uuid.UUID]*domain.Event)
	var mu sync.Mutex

	for _, join := range items {
		// Dedup check if needed, but iterating items implies we want 1-to-1 if possible,
		// though duplicates in map fetch are wasteful.
		// For simplicity, we just fetch all. A better way uses a set of IDs.

		wg.Add(1)
		go func(eid uuid.UUID) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Note: GetEvent is public, so we don't strictly need bearerToken if it's public info.
			// However, if we want personalized info it might matter.
			// Here we use GetEvent similar to GetPublic in router.
			ev, err := h.eventClient.GetEvent(ctx, eid)
			if err == nil && ev != nil {
				mu.Lock()
				eventMap[eid] = ev
				mu.Unlock()
			}
		}(join.EventID)
	}
	wg.Wait()

	// 4. Transform back to EventCard
	// Note: Use join info + event info
	finalItems := make([]domain.EventCard, 0, len(items))
	for _, join := range items {
		ev, found := eventMap[join.EventID]
		if !found {
			// Event might be deleted or service failed. Skip or show placeholder?
			// Skipping is safer to avoid UI crashes.
			continue
		}

		card := domain.EventCard{
			ID:         ev.ID, // Use Event ID not Join ID
			Title:      ev.Title,
			CoverImage: ev.CoverImage,
			StartTime:  ev.StartTime,
			City:       ev.City,
			Category:   ev.Category,
		}
		finalItems = append(finalItems, card)
	}

	payload := domain.PaginatedResponse[domain.EventCard]{
		Items:      finalItems,
		NextCursor: joinRes.NextCursor,
		HasMore:    joinRes.HasMore,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func (h *EventHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var body interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, r, "validation_failed", "invalid request body", http.StatusBadRequest)
		return
	}

	bearerToken := middleware.GetBearerToken(r.Context())
	ev, err := h.eventClient.CreateEvent(r.Context(), bearerToken, body)
	if err != nil {
		handleDownstreamError(w, r, err, "failed to create event")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ev)
}

func (h *EventHandler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		sendError(w, r, "validation_failed", "event id is required", http.StatusBadRequest)
		return
	}

	bearerToken := middleware.GetBearerToken(r.Context())
	ev, err := h.eventClient.PublishEvent(r.Context(), bearerToken, eventID)
	if err != nil {
		handleDownstreamError(w, r, err, "failed to publish event")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ev)
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

	var (
		event    *domain.Event
		eventErr error
		part     *domain.Participation
		partErr  error
		user     *domain.User
		userErr  error
	)

	// 1. Fetch Event (Mandatory)
	{
		ctx, cancel := context.WithTimeout(r.Context(), 1200*time.Millisecond)
		defer cancel()
		event, eventErr = h.eventClient.GetEvent(ctx, eventID)
	}

	if eventErr != nil {
		if eventErr == downstream.ErrNotFound {
			sendError(w, r, "resource_not_found", "event not found", http.StatusNotFound)
			return
		}
		if eventErr == downstream.ErrTimeout {
			sendError(w, r, "upstream_timeout", "event service timeout", http.StatusGatewayTimeout)
			return
		}
		handleDownstreamError(w, r, eventErr, "failed to fetch event")
		return
	}

	// 2. Fetch Participation and Organizer info in parallel
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(r.Context(), 800*time.Millisecond)
		defer cancel()
		part, partErr = h.joinClient.GetParticipation(ctx, eventID, userID, bearerToken)
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(r.Context(), 800*time.Millisecond)
		defer cancel()
		user, userErr = h.authClient.GetUser(ctx, event.OrganizerID)
	}()

	wg.Wait()

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
		part = nil
	}

	if userErr == nil && user != nil {
		event.OrganizerName = user.Email
	} else {
		event.OrganizerName = "Unknown Host"
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
		handleDownstreamError(w, r, err, "failed to join event")
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
		handleDownstreamError(w, r, err, "failed to cancel join")
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
