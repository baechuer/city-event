package feed

import (
	"strings"
	"time"
)

const (
	DefaultLimit    = 20
	MaxLimit        = 100
	StatusPublished = "PUBLISHED"
)

type Event struct {
	EventID        string     `json:"eventId"`
	Title          string     `json:"title"`
	City           string     `json:"city"`
	Venue          string     `json:"venue"`
	StartsAt       *time.Time `json:"startsAt,omitempty"`
	Capacity       int        `json:"capacity"`
	Status         string     `json:"status"`
	ConfirmedCount int        `json:"confirmedCount"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Query struct {
	City   string
	Limit  int
	Offset int
}

func NormalizeQuery(query Query) Query {
	query.City = strings.TrimSpace(query.City)
	if query.Limit <= 0 {
		query.Limit = DefaultLimit
	}
	if query.Limit > MaxLimit {
		query.Limit = MaxLimit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return query
}
