package eventregistration

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/authn"
	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/baechuer/cityevents/internal/platform/httpapi"
	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const eventJSONLimitBytes = 128 * 1024

type Handler struct {
	service *Service
	tokens  authn.TokenManager
}

func NewRouter(ctx context.Context, cfg config.Config, logger *slog.Logger) (http.Handler, func(context.Context) error, error) {
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}

	repo := NewPostgresRepository(pool)
	service := NewService(repo)
	router := NewHTTPHandler(cfg, logger, service)
	cleanup := func(context.Context) error {
		pool.Close()
		return nil
	}
	return router, cleanup, nil
}

func NewHTTPHandler(cfg config.Config, logger *slog.Logger, service *Service) http.Handler {
	r := httpapi.NewBaseRouter(cfg, logger)
	handler := &Handler{
		service: service,
		tokens:  authn.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL),
	}

	r.Post("/v1/events", handler.createEvent)
	r.Get("/v1/events", handler.listEvents)
	r.Get("/v1/events/{eventID}", handler.getEvent)
	r.Patch("/v1/events/{eventID}", handler.updateEvent)
	r.Post("/v1/events/{eventID}/cancel", handler.cancelEvent)
	r.Post("/v1/events/{eventID}/join", handler.joinEvent)
	r.Delete("/v1/events/{eventID}/join", handler.cancelJoin)
	r.Delete("/v1/events/{eventID}/registrations/{userID}", handler.cancelRegistration)
	r.Get("/v1/events/{eventID}/join", handler.getJoinStatus)

	return r
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principalFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	var req createEventRequest
	if err := decodeJSON(w, r, &req, eventJSONLimitBytes); err != nil {
		writeDecodeError(w, err)
		return
	}
	detail, err := h.service.CreateEvent(r.Context(), CreateEventCommand{
		OrganizerID: principal.UserID,
		Role:        principal.Role,
		Title:       req.Title,
		Description: req.Description,
		City:        req.City,
		Venue:       req.Venue,
		StartsAt:    req.StartsAt,
		Capacity:    req.Capacity,
	})
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusCreated, eventDetailResponseFromDomain(detail))
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principalFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	var req updateEventRequest
	if err := decodeJSON(w, r, &req, eventJSONLimitBytes); err != nil {
		writeDecodeError(w, err)
		return
	}
	detail, err := h.service.UpdateEvent(r.Context(), chi.URLParam(r, "eventID"), principal.UserID, principal.Role, UpdateEventCommand{
		Title:       req.Title,
		Description: req.Description,
		City:        req.City,
		Venue:       req.Venue,
		StartsAt:    req.StartsAt,
		Capacity:    req.Capacity,
	})
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, eventDetailResponseFromDomain(detail))
}

func (h *Handler) cancelEvent(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principalFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	detail, err := h.service.CancelEvent(r.Context(), chi.URLParam(r, "eventID"), principal.UserID, principal.Role)
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, eventDetailResponseFromDomain(detail))
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	details, err := h.service.ListPublishedEvents(r.Context())
	if err != nil {
		writeEventError(w, err)
		return
	}
	events := make([]eventDetailResponse, 0, len(details))
	for _, detail := range details {
		events = append(events, eventDetailResponseFromDomain(detail))
	}
	health.WriteJSON(w, http.StatusOK, map[string][]eventDetailResponse{"events": events})
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	viewerID, ok := h.optionalUserIDFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	detail, err := h.service.GetEventDetail(r.Context(), chi.URLParam(r, "eventID"), viewerID)
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, eventDetailResponseFromDomain(detail))
}

func (h *Handler) joinEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userIDFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	result, err := h.service.JoinEvent(r.Context(), chi.URLParam(r, "eventID"), userID, strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, joinResponseFromDomain(result))
}

func (h *Handler) cancelJoin(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userIDFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	result, err := h.service.CancelJoin(r.Context(), chi.URLParam(r, "eventID"), userID)
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, cancelJoinResponseFromDomain(result))
}

func (h *Handler) cancelRegistration(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principalFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	result, err := h.service.CancelRegistration(r.Context(), chi.URLParam(r, "eventID"), principal.UserID, principal.Role, chi.URLParam(r, "userID"))
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, cancelJoinResponseFromDomain(result))
}

func (h *Handler) getJoinStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userIDFromRequest(r)
	if !ok {
		writeEventError(w, ErrUnauthorized)
		return
	}
	status, err := h.service.GetJoinStatus(r.Context(), chi.URLParam(r, "eventID"), userID)
	if err != nil {
		writeEventError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string]RegistrationStatus{"status": status})
}

type createEventRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	City        string    `json:"city"`
	Venue       string    `json:"venue"`
	StartsAt    time.Time `json:"startsAt"`
	Capacity    int       `json:"capacity"`
}

type updateEventRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	City        *string    `json:"city"`
	Venue       *string    `json:"venue"`
	StartsAt    *time.Time `json:"startsAt"`
	Capacity    *int       `json:"capacity"`
}

type eventDetailResponse struct {
	Event            eventResponse      `json:"event"`
	ConfirmedCount   int                `json:"confirmedCount"`
	ViewerJoinStatus RegistrationStatus `json:"viewerJoinStatus,omitempty"`
}

type eventResponse struct {
	ID          string      `json:"id"`
	OrganizerID string      `json:"organizerId"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	City        string      `json:"city"`
	Venue       string      `json:"venue"`
	StartsAt    time.Time   `json:"startsAt"`
	Capacity    int         `json:"capacity"`
	Status      EventStatus `json:"status"`
}

type joinResponse struct {
	RegistrationID   string             `json:"registrationId"`
	EventID          string             `json:"eventId"`
	UserID           string             `json:"userId"`
	Status           RegistrationStatus `json:"status"`
	WaitlistPosition int                `json:"waitlistPosition,omitempty"`
	Existing         bool               `json:"existing"`
}

type cancelJoinResponse struct {
	Status   RegistrationStatus `json:"status"`
	Canceled *joinResponse      `json:"canceled,omitempty"`
	Promoted *joinResponse      `json:"promoted,omitempty"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func eventDetailResponseFromDomain(detail EventDetail) eventDetailResponse {
	return eventDetailResponse{
		Event: eventResponse{
			ID:          detail.Event.ID,
			OrganizerID: detail.Event.OrganizerID,
			Title:       detail.Event.Title,
			Description: detail.Event.Description,
			City:        detail.Event.City,
			Venue:       detail.Event.Venue,
			StartsAt:    detail.Event.StartsAt,
			Capacity:    detail.Event.Capacity,
			Status:      detail.Event.Status,
		},
		ConfirmedCount:   detail.ConfirmedCount,
		ViewerJoinStatus: detail.ViewerJoinStatus,
	}
}

func joinResponseFromDomain(result JoinResult) joinResponse {
	return joinResponse{
		RegistrationID:   result.Registration.ID,
		EventID:          result.Registration.EventID,
		UserID:           result.Registration.UserID,
		Status:           result.Registration.Status,
		WaitlistPosition: result.Registration.WaitlistPosition,
		Existing:         result.Existing,
	}
}

func registrationResponse(reg Registration) joinResponse {
	return joinResponse{
		RegistrationID:   reg.ID,
		EventID:          reg.EventID,
		UserID:           reg.UserID,
		Status:           reg.Status,
		WaitlistPosition: reg.WaitlistPosition,
	}
}

func cancelJoinResponseFromDomain(result CancelJoinResult) cancelJoinResponse {
	resp := cancelJoinResponse{Status: result.Status}
	if result.Canceled != nil {
		canceled := registrationResponse(*result.Canceled)
		resp.Canceled = &canceled
	}
	if result.Promoted != nil {
		promoted := registrationResponse(*result.Promoted)
		resp.Promoted = &promoted
	}
	return resp
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	return httpapi.DecodeJSONLimited(w, r, target, maxBytes)
}

type principal struct {
	UserID string
	Role   identity.Role
}

func (h *Handler) principalFromRequest(r *http.Request) (principal, bool) {
	token, ok := authn.BearerToken(r)
	if !ok {
		return principal{}, false
	}
	claims, err := h.tokens.Verify(token)
	if err != nil {
		return principal{}, false
	}
	return principal{UserID: claims.UserID, Role: claims.Role}, true
}

func (h *Handler) userIDFromRequest(r *http.Request) (string, bool) {
	principal, ok := h.principalFromRequest(r)
	return principal.UserID, ok
}

func (h *Handler) optionalUserIDFromRequest(r *http.Request) (string, bool) {
	token, ok := authn.BearerToken(r)
	if !ok {
		return "", true
	}
	claims, err := h.tokens.Verify(token)
	if err != nil {
		return "", false
	}
	return claims.UserID, true
}

func writeEventError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "user identity required")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "operation is not allowed")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, ErrInvalidEvent), errors.Is(err, ErrInvalidCapacity), errors.Is(err, ErrEventStarted):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrCapacityBelowConfirmed):
		writeError(w, http.StatusConflict, "capacity_below_confirmed", err.Error())
	case errors.Is(err, ErrEventCanceled):
		writeError(w, http.StatusConflict, "event_canceled", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, httpapi.ErrRequestBodyTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	health.WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
