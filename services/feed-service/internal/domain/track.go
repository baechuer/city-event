package domain

import (
	"time"

	"github.com/google/uuid"
)

// TrackEvent represents a user behavior event
type TrackEvent struct {
	ID         uuid.UUID
	ActorKey   string // "u:<user_id>" or "a:<anon_id>"
	EventType  string // "impression", "view", "join"
	EventID    uuid.UUID
	FeedType   string
	Position   int
	RequestID  string
	BucketDate time.Time
	OccurredAt time.Time
}

// TrackRequest is the API request for /track
type TrackRequest struct {
	EventType string `json:"event_type"` // "impression", "view"
	EventID   string `json:"event_id"`
	FeedType  string `json:"feed_type,omitempty"`
	Position  int    `json:"position,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}
