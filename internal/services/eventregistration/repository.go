package eventregistration

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

type Repository interface {
	CreateEvent(context.Context, Event) (EventDetail, error)
	UpdateEvent(context.Context, string, string, UpdateEventCommand, time.Time) (EventDetail, error)
	CancelEvent(context.Context, string, string, time.Time) (EventDetail, error)
	ListPublishedEvents(context.Context) ([]EventDetail, error)
	GetEventDetail(context.Context, string, string) (EventDetail, error)
	JoinEvent(context.Context, string, string, string, time.Time) (JoinResult, error)
	CancelJoin(context.Context, string, string, time.Time) (CancelJoinResult, error)
	GetJoinStatus(context.Context, string, string) (RegistrationStatus, error)
}

type OutboxMessage struct {
	ID            string
	AggregateType string
	AggregateID   string
	RoutingKey    string
	Payload       map[string]any
	Status        string
	CreatedAt     time.Time
	AvailableAt   time.Time
	SentAt        *time.Time
}

type MemoryRepository struct {
	mu            sync.Mutex
	events        map[string]Event
	registrations map[string]Registration
	outbox        []OutboxMessage
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		events:        map[string]Event{},
		registrations: map[string]Registration{},
		outbox:        []OutboxMessage{},
	}
}

func (r *MemoryRepository) CreateEvent(_ context.Context, event Event) (EventDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events[event.ID] = event
	r.addOutbox("event", event.ID, RoutingEventPublished, eventPayload(event), event.CreatedAt)
	return r.detailLocked(event.ID, ""), nil
}

func (r *MemoryRepository) UpdateEvent(_ context.Context, eventID, organizerID string, cmd UpdateEventCommand, now time.Time) (EventDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	event, ok := r.events[eventID]
	if !ok {
		return EventDetail{}, ErrNotFound
	}
	if event.OrganizerID != organizerID {
		return EventDetail{}, ErrForbidden
	}
	if event.Status == EventStatusCanceled {
		return EventDetail{}, ErrEventCanceled
	}

	confirmed := r.confirmedCountLocked(eventID)
	if cmd.Title != nil {
		event.Title = *cmd.Title
	}
	if cmd.Description != nil {
		event.Description = *cmd.Description
	}
	if cmd.City != nil {
		event.City = *cmd.City
	}
	if cmd.Venue != nil {
		event.Venue = *cmd.Venue
	}
	if cmd.StartsAt != nil {
		event.StartsAt = cmd.StartsAt.UTC()
	}
	if cmd.Capacity != nil {
		if err := ValidateCapacityChange(*cmd.Capacity, confirmed); err != nil {
			return EventDetail{}, err
		}
		event.Capacity = *cmd.Capacity
	}
	event.UpdatedAt = now.UTC()
	if err := event.Validate(now); err != nil {
		return EventDetail{}, err
	}

	r.events[eventID] = event
	r.addOutbox("event", event.ID, RoutingEventUpdated, eventPayload(event), now)
	return r.detailLocked(eventID, ""), nil
}

func (r *MemoryRepository) CancelEvent(_ context.Context, eventID, organizerID string, now time.Time) (EventDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	event, ok := r.events[eventID]
	if !ok {
		return EventDetail{}, ErrNotFound
	}
	if event.OrganizerID != organizerID {
		return EventDetail{}, ErrForbidden
	}
	if event.Status == EventStatusCanceled {
		return r.detailLocked(eventID, ""), nil
	}
	event.Status = EventStatusCanceled
	event.UpdatedAt = now.UTC()
	r.events[eventID] = event
	for id, reg := range r.registrations {
		if reg.EventID == eventID && (reg.Status == RegistrationStatusConfirmed || reg.Status == RegistrationStatusWaitlisted) {
			reg.Status = RegistrationStatusCanceled
			reg.UpdatedAt = now.UTC()
			r.registrations[id] = reg
		}
	}
	r.addOutbox("event", event.ID, RoutingEventCanceled, eventPayload(event), now)
	return r.detailLocked(eventID, ""), nil
}

func (r *MemoryRepository) ListPublishedEvents(_ context.Context) ([]EventDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.events))
	for id, event := range r.events {
		if event.Status == EventStatusPublished {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := r.events[ids[i]], r.events[ids[j]]
		if a.StartsAt.Equal(b.StartsAt) {
			return a.ID < b.ID
		}
		return a.StartsAt.Before(b.StartsAt)
	})
	details := make([]EventDetail, 0, len(ids))
	for _, id := range ids {
		details = append(details, r.detailLocked(id, ""))
	}
	return details, nil
}

func (r *MemoryRepository) GetEventDetail(_ context.Context, eventID, viewerID string) (EventDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.events[eventID]; !ok {
		return EventDetail{}, ErrNotFound
	}
	return r.detailLocked(eventID, viewerID), nil
}

func (r *MemoryRepository) JoinEvent(_ context.Context, eventID, userID, idempotencyKey string, now time.Time) (JoinResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	event, ok := r.events[eventID]
	if !ok {
		return JoinResult{}, ErrNotFound
	}
	if existing, ok := r.activeRegistrationLocked(eventID, userID); ok {
		return JoinResult{Registration: existing, Existing: true}, nil
	}
	status, err := DecideJoinStatus(event, r.confirmedCountLocked(eventID), now)
	if err != nil {
		return JoinResult{}, err
	}

	reg := Registration{
		ID:             NewID(),
		EventID:        eventID,
		UserID:         userID,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
	if status == RegistrationStatusWaitlisted {
		reg.WaitlistPosition = r.nextWaitlistPositionLocked(eventID)
	}
	r.registrations[reg.ID] = reg

	routing := RoutingJoinConfirmed
	if status == RegistrationStatusWaitlisted {
		routing = RoutingJoinWaitlisted
	}
	r.addOutbox("registration", reg.ID, routing, registrationPayload(reg), now)
	return JoinResult{Registration: reg}, nil
}

func (r *MemoryRepository) CancelJoin(_ context.Context, eventID, userID string, now time.Time) (CancelJoinResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.events[eventID]; !ok {
		return CancelJoinResult{}, ErrNotFound
	}
	reg, ok := r.activeRegistrationLocked(eventID, userID)
	if !ok {
		status := r.latestStatusLocked(eventID, userID)
		return CancelJoinResult{Status: status, AlreadyNoActiveReg: true}, nil
	}

	oldStatus := reg.Status
	reg.Status = RegistrationStatusCanceled
	reg.UpdatedAt = now.UTC()
	r.registrations[reg.ID] = reg
	r.addOutbox("registration", reg.ID, RoutingJoinCanceled, registrationPayload(reg), now)

	result := CancelJoinResult{Status: RegistrationStatusCanceled, Canceled: &reg}
	if oldStatus == RegistrationStatusConfirmed {
		if promoted, ok := r.firstWaitlistedLocked(eventID); ok {
			promoted.Status = RegistrationStatusConfirmed
			promoted.WaitlistPosition = 0
			promoted.UpdatedAt = now.UTC()
			r.registrations[promoted.ID] = promoted
			r.addOutbox("registration", promoted.ID, RoutingJoinPromoted, registrationPayload(promoted), now)
			result.Promoted = &promoted
		}
	}
	return result, nil
}

func (r *MemoryRepository) GetJoinStatus(_ context.Context, eventID, userID string) (RegistrationStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.events[eventID]; !ok {
		return "", ErrNotFound
	}
	return r.latestStatusLocked(eventID, userID), nil
}

func (r *MemoryRepository) CountOutboxByRoutingKey(routingKey string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, msg := range r.outbox {
		if msg.RoutingKey == routingKey {
			count++
		}
	}
	return count
}

func (r *MemoryRepository) ConfirmedCount(eventID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.confirmedCountLocked(eventID)
}

func (r *MemoryRepository) WaitlistedCount(eventID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.Status == RegistrationStatusWaitlisted {
			count++
		}
	}
	return count
}

func (r *MemoryRepository) ActiveRegistrationCount(eventID, userID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.UserID == userID && (reg.Status == RegistrationStatusConfirmed || reg.Status == RegistrationStatusWaitlisted) {
			count++
		}
	}
	return count
}

func (r *MemoryRepository) detailLocked(eventID, viewerID string) EventDetail {
	event := r.events[eventID]
	status := RegistrationStatusNotJoined
	if viewerID != "" {
		status = r.latestStatusLocked(eventID, viewerID)
	}
	return EventDetail{
		Event:            event,
		ConfirmedCount:   r.confirmedCountLocked(eventID),
		ViewerJoinStatus: status,
	}
}

func (r *MemoryRepository) confirmedCountLocked(eventID string) int {
	count := 0
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.Status == RegistrationStatusConfirmed {
			count++
		}
	}
	return count
}

func (r *MemoryRepository) activeRegistrationLocked(eventID, userID string) (Registration, bool) {
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.UserID == userID && (reg.Status == RegistrationStatusConfirmed || reg.Status == RegistrationStatusWaitlisted) {
			return reg, true
		}
	}
	return Registration{}, false
}

func (r *MemoryRepository) latestStatusLocked(eventID, userID string) RegistrationStatus {
	latest := Registration{}
	found := false
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.UserID == userID {
			if !found || reg.UpdatedAt.After(latest.UpdatedAt) {
				latest = reg
				found = true
			}
		}
	}
	if !found {
		return RegistrationStatusNotJoined
	}
	return latest.Status
}

func (r *MemoryRepository) nextWaitlistPositionLocked(eventID string) int {
	maxPos := 0
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.WaitlistPosition > maxPos {
			maxPos = reg.WaitlistPosition
		}
	}
	return maxPos + 1
}

func (r *MemoryRepository) firstWaitlistedLocked(eventID string) (Registration, bool) {
	waitlisted := make([]Registration, 0)
	for _, reg := range r.registrations {
		if reg.EventID == eventID && reg.Status == RegistrationStatusWaitlisted {
			waitlisted = append(waitlisted, reg)
		}
	}
	if len(waitlisted) == 0 {
		return Registration{}, false
	}
	sort.Slice(waitlisted, func(i, j int) bool {
		if waitlisted[i].WaitlistPosition == waitlisted[j].WaitlistPosition {
			return waitlisted[i].CreatedAt.Before(waitlisted[j].CreatedAt)
		}
		return waitlisted[i].WaitlistPosition < waitlisted[j].WaitlistPosition
	})
	return waitlisted[0], true
}

func (r *MemoryRepository) addOutbox(aggregateType, aggregateID, routingKey string, payload map[string]any, now time.Time) {
	r.outbox = append(r.outbox, OutboxMessage{
		ID:            NewID(),
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		RoutingKey:    routingKey,
		Payload:       payload,
		Status:        "PENDING",
		CreatedAt:     now.UTC(),
		AvailableAt:   now.UTC(),
	})
}

func eventPayload(event Event) map[string]any {
	return map[string]any{
		"eventId":     event.ID,
		"organizerId": event.OrganizerID,
		"title":       event.Title,
		"city":        event.City,
		"venue":       event.Venue,
		"startsAt":    event.StartsAt,
		"capacity":    event.Capacity,
		"status":      event.Status,
	}
}

func registrationPayload(reg Registration) map[string]any {
	return map[string]any{
		"registrationId":   reg.ID,
		"eventId":          reg.EventID,
		"userId":           reg.UserID,
		"status":           reg.Status,
		"waitlistPosition": reg.WaitlistPosition,
	}
}

func marshalPayload(payload map[string]any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errors.New("empty payload")
	}
	return raw, nil
}
