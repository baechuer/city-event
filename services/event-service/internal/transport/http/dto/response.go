package dto

import "time"

// EventResp is the stable API response model.
// NOTE: derived fields (ended/joinable) are computed at runtime (not stored in DB).
type EventResp struct {
	ID      string `json:"id"`
	OwnerID string `json:"owner_id"`

	Title       string `json:"title"`
	Description string `json:"description"`
	City        string `json:"city"`
	Category    string `json:"category"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// 0 means unlimited
	Capacity           int `json:"capacity"`
	ActiveParticipants int `json:"active_participants"`

	Status string `json:"status"`

	PublishedAt *time.Time `json:"published_at,omitempty"`
	CanceledAt  *time.Time `json:"canceled_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Derived
	Ended    bool `json:"ended"`
	Joinable bool `json:"joinable"`
}

type PageResp[T any] struct {
	Items      []T    `json:"items"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	Total      int    `json:"total"`
	NextCursor string `json:"next_cursor,omitempty"`
}
