package eventregistration

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type EventStatus string

const (
	EventStatusPublished EventStatus = "PUBLISHED"
	EventStatusCanceled  EventStatus = "CANCELED"
)

type RegistrationStatus string

const (
	RegistrationStatusNotJoined  RegistrationStatus = "NOT_JOINED"
	RegistrationStatusConfirmed  RegistrationStatus = "CONFIRMED"
	RegistrationStatusWaitlisted RegistrationStatus = "WAITLISTED"
	RegistrationStatusCanceled   RegistrationStatus = "CANCELED"
)

const (
	RoutingEventPublished = "event.published"
	RoutingEventUpdated   = "event.updated"
	RoutingEventCanceled  = "event.canceled"
	RoutingJoinConfirmed  = "join.confirmed"
	RoutingJoinWaitlisted = "join.waitlisted"
	RoutingJoinCanceled   = "join.canceled"
	RoutingJoinPromoted   = "join.promoted"
)

var (
	ErrUnauthorized           = errors.New("user identity required")
	ErrForbidden              = errors.New("forbidden")
	ErrNotFound               = errors.New("not found")
	ErrInvalidEvent           = errors.New("invalid event")
	ErrInvalidCapacity        = errors.New("invalid capacity")
	ErrCapacityBelowConfirmed = errors.New("capacity below confirmed registrations")
	ErrEventCanceled          = errors.New("event canceled")
	ErrEventStarted           = errors.New("event already started")
)

type Event struct {
	ID          string
	OrganizerID string
	Title       string
	Description string
	City        string
	Venue       string
	StartsAt    time.Time
	Capacity    int
	Status      EventStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Registration struct {
	ID               string
	EventID          string
	UserID           string
	Status           RegistrationStatus
	WaitlistPosition int
	IdempotencyKey   string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type EventDetail struct {
	Event            Event
	ConfirmedCount   int
	ViewerJoinStatus RegistrationStatus
}

type JoinResult struct {
	Registration Registration
	Existing     bool
}

type CancelJoinResult struct {
	Status             RegistrationStatus
	Canceled           *Registration
	Promoted           *Registration
	AlreadyNoActiveReg bool
}

func NewEvent(organizerID, title, description, city, venue string, startsAt time.Time, capacity int, now time.Time) (Event, error) {
	event := Event{
		ID:          NewID(),
		OrganizerID: strings.TrimSpace(organizerID),
		Title:       strings.TrimSpace(title),
		Description: strings.TrimSpace(description),
		City:        strings.TrimSpace(city),
		Venue:       strings.TrimSpace(venue),
		StartsAt:    startsAt.UTC(),
		Capacity:    capacity,
		Status:      EventStatusPublished,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}
	if err := event.Validate(now); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (e Event) Validate(now time.Time) error {
	if strings.TrimSpace(e.OrganizerID) == "" {
		return ErrUnauthorized
	}
	if strings.TrimSpace(e.Title) == "" {
		return ErrInvalidEvent
	}
	if strings.TrimSpace(e.City) == "" {
		return ErrInvalidEvent
	}
	if strings.TrimSpace(e.Venue) == "" {
		return ErrInvalidEvent
	}
	if !e.StartsAt.After(now) {
		return ErrEventStarted
	}
	if e.Capacity <= 0 {
		return ErrInvalidCapacity
	}
	if e.Status != EventStatusPublished && e.Status != EventStatusCanceled {
		return ErrInvalidEvent
	}
	return nil
}

func ValidateCapacityChange(newCapacity, confirmedCount int) error {
	if newCapacity <= 0 {
		return ErrInvalidCapacity
	}
	if newCapacity < confirmedCount {
		return ErrCapacityBelowConfirmed
	}
	return nil
}

func DecideJoinStatus(event Event, confirmedCount int, now time.Time) (RegistrationStatus, error) {
	if event.Status == EventStatusCanceled {
		return "", ErrEventCanceled
	}
	if !event.StartsAt.After(now) {
		return "", ErrEventStarted
	}
	if confirmedCount < event.Capacity {
		return RegistrationStatusConfirmed, nil
	}
	return RegistrationStatusWaitlisted, nil
}

func NewID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
