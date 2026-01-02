package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type JoinStatus string

const (
	StatusActive     JoinStatus = "active"
	StatusWaitlisted JoinStatus = "waitlisted"
	StatusCanceled   JoinStatus = "canceled"
	StatusExpired    JoinStatus = "expired"
	StatusRejected   JoinStatus = "rejected"
)

var (
	ErrEventNotFound = errors.New("event not found") // for shared-db lookup or snapshot missing
	ErrEventClosed   = errors.New("event is closed")
	ErrEventFull     = errors.New("event is full")

	ErrForbidden = errors.New("forbidden")
	ErrBanned    = errors.New("user is banned from this event")

	ErrCacheMiss     = errors.New("cache miss")
	ErrAlreadyJoined = errors.New("already joined event")
	ErrEventNotKnown = errors.New("unknown event") // capacity row missing (your current join path)
	ErrNotJoined     = errors.New("event not joined")

	// Idempotency
	ErrIdempotencyKeyMismatch = errors.New("idempotency key mismatch")
)

type KeysetCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type JoinRecord struct {
	ID uuid.UUID `json:"id"`

	EventID uuid.UUID  `json:"event_id"`
	UserID  uuid.UUID  `json:"user_id"`
	Status  JoinStatus `json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	CanceledAt    *time.Time `json:"canceled_at,omitempty"`
	ExpiredAt     *time.Time `json:"expired_at,omitempty"`
	ExpiredReason *string    `json:"expired_reason,omitempty"`

	CanceledBy     *uuid.UUID `json:"canceled_by,omitempty"`
	CanceledReason *string    `json:"canceled_reason,omitempty"`

	RejectedAt     *time.Time `json:"rejected_at,omitempty"`
	RejectedBy     *uuid.UUID `json:"rejected_by,omitempty"`
	RejectedReason *string    `json:"rejected_reason,omitempty"`
}

type EventStats struct {
	EventID       uuid.UUID `json:"event_id"`
	Capacity      int       `json:"capacity"`
	ActiveCount   int       `json:"active_count"`
	WaitlistCount int       `json:"waitlist_count"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// JoinRepository handles DB transactions, locking, outbox, and read endpoints.
type JoinRepository interface {
	JoinEvent(ctx context.Context, traceID, idempotencyKey string, eventID, userID uuid.UUID) (JoinStatus, error)
	CancelJoin(ctx context.Context, traceID, idempotencyKey string, eventID, userID uuid.UUID) error

	// Single Check
	GetByEventAndUser(ctx context.Context, eventID, userID uuid.UUID) (JoinRecord, error)

	// ACL on shared DB
	GetEventOwnerID(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error)

	// Reads
	ListMyJoins(ctx context.Context, userID uuid.UUID, statuses []JoinStatus, from, to *time.Time, limit int, cursor *KeysetCursor) ([]JoinRecord, *KeysetCursor, error)
	ListParticipants(ctx context.Context, eventID uuid.UUID, limit int, cursor *KeysetCursor) ([]JoinRecord, *KeysetCursor, error) // active
	ListWaitlist(ctx context.Context, eventID uuid.UUID, limit int, cursor *KeysetCursor) ([]JoinRecord, *KeysetCursor, error)     // waitlisted
	GetStats(ctx context.Context, eventID uuid.UUID) (EventStats, error)

	// Moderation
	Kick(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string) error
	Ban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string, expiresAt *time.Time) error
	Unban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID) error

	// Existing
	InitCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error
	HandleEventCanceled(ctx context.Context, traceID string, eventID uuid.UUID, reason string) error
}

type CacheRepository interface {
	GetEventCapacity(ctx context.Context, eventID uuid.UUID) (int, error)
	SetEventCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error

	AllowRequest(ctx context.Context, ip string, limit int, window time.Duration) (bool, error)
}
