package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAlreadyJoined          = errors.New("already_joined")
	ErrIdempotencyKeyMismatch = errors.New("idempotency_key_mismatch")
)

type Event struct {
	ID                 uuid.UUID `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	City               string    `json:"city"`
	Category           string    `json:"category"`
	CoverImage         string    `json:"cover_image,omitempty"`
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	Location           string    `json:"location"`
	Capacity           int       `json:"capacity"`
	ActiveParticipants int       `json:"active_participants"`
	CreatedBy          uuid.UUID `json:"created_by"` // Deprecated?
	OwnerID            uuid.UUID `json:"owner_id"`
	OrganizerName      string    `json:"organizer_name,omitempty"`
}

type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
}

type ParticipationStatus string

type EventCard struct {
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	CoverImage string    `json:"cover_image,omitempty"`
	StartTime  time.Time `json:"start_time"`
	City       string    `json:"city"`
	Category   string    `json:"category"`
}

type PaginatedResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

const (
	StatusNone       ParticipationStatus = "none"
	StatusActive     ParticipationStatus = "active"
	StatusWaitlisted ParticipationStatus = "waitlisted"
	StatusCanceled   ParticipationStatus = "canceled"
	StatusRejected   ParticipationStatus = "rejected"
	StatusExpired    ParticipationStatus = "expired"
)

type JoinRecord struct {
	ID        uuid.UUID `json:"id"`
	EventID   uuid.UUID `json:"event_id"`
	UserID    uuid.UUID `json:"user_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Participation struct {
	EventID  uuid.UUID           `json:"event_id"`
	UserID   uuid.UUID           `json:"user_id"`
	Status   ParticipationStatus `json:"status"`
	JoinedAt *time.Time          `json:"joined_at,omitempty"`
}

type ActionPolicy struct {
	CanJoin   bool   `json:"can_join"`
	CanCancel bool   `json:"can_cancel"`
	Reason    string `json:"reason,omitempty"`
}

type APIError struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}
