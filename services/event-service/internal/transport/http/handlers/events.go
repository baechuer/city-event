package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/dto"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/middleware"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/response"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/validate"
)

type Clock interface{ Now() time.Time }

type EventsHandler struct {
	svc   *event.Service
	clock Clock
}

func NewEventsHandler(svc *event.Service, clock Clock) *EventsHandler {
	return &EventsHandler{svc: svc, clock: clock}
}

// -------------------------
// Public
// -------------------------

func (h *EventsHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// page is legacy (offset). We keep it for compatibility but ignore it for keyset behavior.
	_, _ = strconv.Atoi(q.Get("page"))

	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	sort := strings.TrimSpace(q.Get("sort"))
	cursor := strings.TrimSpace(q.Get("cursor"))

	// time range
	var fromPtr, toPtr *time.Time
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		t, err := parseRFC3339OrNano(v)
		if err != nil {
			response.Err(w, r, domain.ErrValidationMeta("invalid query param", map[string]string{
				"from": "must be RFC3339 timestamp",
			}))
			return
		}
		tt := t.UTC()
		fromPtr = &tt
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		t, err := parseRFC3339OrNano(v)
		if err != nil {
			response.Err(w, r, domain.ErrValidationMeta("invalid query param", map[string]string{
				"to": "must be RFC3339 timestamp",
			}))
			return
		}
		tt := t.UTC()
		toPtr = &tt
	}

	// exclude_expired
	excludeExpired := false
	if v := q.Get("exclude_expired"); v != "" {
		excludeExpired, _ = strconv.ParseBool(v)
	}

	filter := event.ListFilter{
		City:           q.Get("city"),
		Query:          q.Get("q"),
		Category:       q.Get("category"),
		From:           fromPtr,
		To:             toPtr,
		PageSize:       pageSize,
		Sort:           sort,
		Cursor:         cursor,
		ExcludeExpired: excludeExpired,
	}

	res, err := h.svc.ListPublic(r.Context(), filter)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	out := make([]dto.EventResp, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, dto.ToEventResp(it, now))
	}

	response.Data(w, http.StatusOK, dto.PageResp[dto.EventResp]{
		Items:      out,
		NextCursor: res.NextCursor,
		Page:       1,
		PageSize:   filter.PageSize,
		Total:      0, // keyset 模式建议不返回 total（避免 count 慢）
	})
}

func (h *EventsHandler) GetPublic(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	if !validate.IsUUID(id) {
		response.Err(w, r, domain.ErrValidationMeta("invalid path param", map[string]string{
			"event_id": "must be uuid",
		}))
		return
	}

	ev, err := h.svc.GetPublic(r.Context(), id)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusOK, dto.ToEventResp(ev, now))
}

// -------------------------
// Organizer
// -------------------------

func (h *EventsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateEventReq
	if err := validate.DecodeJSON(r, &req); err != nil {
		response.Err(w, r, domain.ErrValidationMeta("invalid json body", map[string]string{
			"body": "malformed JSON or invalid fields",
		}))
		return
	}

	cmd := event.CreateCmd{
		ActorID:       middleware.UserID(r),
		ActorRole:     middleware.Role(r),
		Title:         req.Title,
		Description:   req.Description,
		City:          req.City,
		Category:      req.Category,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Capacity:      req.Capacity,
		CoverImageIDs: req.CoverImageIDs,
	}

	ev, err := h.svc.Create(r.Context(), cmd)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusCreated, dto.ToEventResp(ev, now))
}

func (h *EventsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	if !validate.IsUUID(id) {
		response.Err(w, r, domain.ErrValidationMeta("invalid path param", map[string]string{
			"event_id": "must be uuid",
		}))
		return
	}

	var req dto.UpdateEventReq
	if err := validate.DecodeJSON(r, &req); err != nil {
		response.Err(w, r, domain.ErrValidationMeta("invalid json body", map[string]string{
			"body": "malformed JSON or invalid fields",
		}))
		return
	}

	cmd := event.UpdateCmd{
		ActorID:       middleware.UserID(r),
		ActorRole:     middleware.Role(r),
		EventID:       id,
		Title:         req.Title,
		Description:   req.Description,
		City:          req.City,
		Category:      req.Category,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Capacity:      req.Capacity,
		CoverImageIDs: req.CoverImageIDs,
	}

	ev, err := h.svc.Update(r.Context(), cmd)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusOK, dto.ToEventResp(ev, now))
}

func (h *EventsHandler) Publish(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	if !validate.IsUUID(id) {
		response.Err(w, r, domain.ErrValidationMeta("invalid path param", map[string]string{
			"event_id": "must be uuid",
		}))
		return
	}

	ev, err := h.svc.Publish(r.Context(), id, middleware.UserID(r), middleware.Role(r))
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusOK, dto.ToEventResp(ev, now))
}

func (h *EventsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	if !validate.IsUUID(id) {
		response.Err(w, r, domain.ErrValidationMeta("invalid path param", map[string]string{
			"event_id": "must be uuid",
		}))
		return
	}

	var req dto.CancelEventReq
	if err := validate.DecodeJSON(r, &req); err != nil {
		// Fallback for requests without body (e.g. legacy or internal calls that don't need reason)
		req.Reason = ""
	}

	ev, err := h.svc.Cancel(r.Context(), id, middleware.UserID(r), middleware.Role(r), req.Reason)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusOK, dto.ToEventResp(ev, now))
}

func (h *EventsHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := strings.TrimSpace(q.Get("status"))
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	items, total, err := h.svc.ListMyEvents(r.Context(), middleware.UserID(r), middleware.Role(r), status, page, pageSize)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	out := make([]dto.EventResp, 0, len(items))
	for _, it := range items {
		out = append(out, dto.ToEventResp(it, now))
	}

	response.Data(w, http.StatusOK, dto.PageResp[dto.EventResp]{
		Items:    out,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

func (h *EventsHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	if !validate.IsUUID(id) {
		response.Err(w, r, domain.ErrValidationMeta("invalid path param", map[string]string{
			"event_id": "must be uuid",
		}))
		return
	}

	ev, err := h.svc.GetForOwner(r.Context(), id, middleware.UserID(r), middleware.Role(r))
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusOK, dto.ToEventResp(ev, now))
}

// -------------------------
// Meta / Autocomplete
// -------------------------

// GetCitySuggestions returns city suggestions for autocomplete.
func (h *EventsHandler) GetCitySuggestions(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Validation: minimum length
	if len([]rune(query)) < 2 {
		response.Data(w, http.StatusOK, []string{})
		return
	}

	// Validation: maximum length (prevent abuse)
	if len(query) > 64 {
		query = query[:64]
	}

	cities, err := h.svc.GetCitySuggestions(r.Context(), query, 10)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	// Return empty array if no results (not null)
	if cities == nil {
		cities = []string{}
	}

	response.Data(w, http.StatusOK, cities)
}

func parseRFC3339OrNano(s string) (time.Time, error) {
	// accept both RFC3339 and RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func (h *EventsHandler) Unpublish(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "event_id")
	if !validate.IsUUID(id) {
		response.Err(w, r, domain.ErrValidationMeta("invalid path param", map[string]string{
			"event_id": "must be uuid",
		}))
		return
	}

	var req dto.UnpublishEventReq
	if err := validate.DecodeJSON(r, &req); err != nil {
		req.Reason = ""
	}

	actorID := middleware.UserID(r)
	actorRole := middleware.Role(r)

	ev, err := h.svc.Unpublish(r.Context(), id, actorID, actorRole, req.Reason)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	now := h.clock.Now().UTC()
	response.Data(w, http.StatusOK, dto.ToEventResp(ev, now))
}

// GetBatch returns multiple events by their IDs.
// Used by BFF to avoid N+1 queries when enriching join records.
// POST /event/v1/events/batch
// Body: {"event_ids": ["uuid1", "uuid2", ...]}
// Response: {"data": {"uuid1": {...}, "uuid2": {...}}}
func (h *EventsHandler) GetBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventIDs []string `json:"event_ids"`
	}
	if err := validate.DecodeJSON(r, &req); err != nil {
		response.Err(w, r, domain.ErrValidationMeta("invalid json body", map[string]string{
			"body": "malformed JSON or invalid fields",
		}))
		return
	}

	// Validate max batch size
	if len(req.EventIDs) > 50 {
		response.Err(w, r, domain.ErrValidationMeta("batch too large", map[string]string{
			"event_ids": "max 50 events per batch",
		}))
		return
	}

	// Validate each ID is a UUID
	for _, id := range req.EventIDs {
		if !validate.IsUUID(id) {
			response.Err(w, r, domain.ErrValidationMeta("invalid event_id", map[string]string{
				"event_ids": "all IDs must be valid UUIDs",
			}))
			return
		}
	}

	events, err := h.svc.GetBatch(r.Context(), req.EventIDs)
	if err != nil {
		response.Err(w, r, err)
		return
	}

	// Convert to map[event_id]EventResp for easy client-side lookup
	now := h.clock.Now().UTC()
	result := make(map[string]dto.EventResp)
	for _, ev := range events {
		result[ev.ID] = dto.ToEventResp(ev, now)
	}

	response.Data(w, http.StatusOK, result)
}
