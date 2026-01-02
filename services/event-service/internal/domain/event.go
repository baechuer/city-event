package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID                 string
	OwnerID            string
	Title              string
	Description        string
	City               string
	Category           string
	StartTime          time.Time
	EndTime            time.Time
	Capacity           int // 0 = unlimited
	ActiveParticipants int

	Status      EventStatus
	PublishedAt *time.Time
	CanceledAt  *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewDraft(ownerID, title, description, city, category string, start, end time.Time, capacity int, now time.Time) (*Event, error) {
	ownerID = strings.TrimSpace(ownerID)
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	city = strings.TrimSpace(city)
	category = strings.TrimSpace(category)

	if ownerID == "" {
		return nil, ErrValidation("owner_id is required")
	}
	if title == "" || len(title) > 120 {
		return nil, ErrValidation("title is required and must be <= 120 chars")
	}
	if description == "" || len(description) > 4000 {
		return nil, ErrValidation("description is required and must be <= 4000 chars")
	}
	if city == "" || len(city) > 80 {
		return nil, ErrValidation("city is required and must be <= 80 chars")
	}
	if category == "" || len(category) > 80 {
		return nil, ErrValidation("category is required and must be <= 80 chars")
	}
	if end.IsZero() || start.IsZero() || !end.After(start) {
		return nil, ErrValidation("end_time must be after start_time")
	}
	if capacity < 0 {
		return nil, ErrValidation("capacity must be >= 0 (0 means unlimited)")
	}

	return &Event{
		ID:          uuid.NewString(),
		OwnerID:     ownerID,
		Title:       title,
		Description: description,
		City:        city,
		Category:    category,
		StartTime:   start.UTC(),
		EndTime:     end.UTC(),
		Capacity:    capacity,
		Status:      StatusDraft,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}, nil
}

func (e *Event) IsEnded(now time.Time) bool {
	return !now.Before(e.EndTime) // now >= end_time => ended
}

func (e *Event) Publish(now time.Time) error {
	if e.Status != StatusDraft {
		return ErrInvalidState("only draft can be published")
	}
	if !e.StartTime.After(now) {
		return ErrValidation("cannot publish an event that starts in the past")
	}
	t := now.UTC()
	e.Status = StatusPublished
	e.PublishedAt = &t
	e.UpdatedAt = t
	return nil
}

func (e *Event) Cancel(now time.Time) error {
	if e.Status == StatusCanceled {
		return ErrInvalidState("event already canceled")
	}
	if e.IsEnded(now) {
		return ErrInvalidState("cannot cancel an ended event")
	}
	t := now.UTC()
	e.Status = StatusCanceled
	e.CanceledAt = &t
	e.UpdatedAt = t
	return nil
}

// MVP: allow update in draft/published (but not canceled/ended)
func (e *Event) ApplyUpdate(title, description, city, category *string, start, end *time.Time, capacity *int, now time.Time) error {
	if e.Status == StatusCanceled {
		return ErrInvalidState("canceled event cannot be updated")
	}
	if e.IsEnded(now) {
		return ErrInvalidState("ended event cannot be updated")
	}

	if title != nil {
		v := strings.TrimSpace(*title)
		if v == "" || len(v) > 120 {
			return ErrValidation("title must be non-empty and <= 120 chars")
		}
		e.Title = v
	}
	if description != nil {
		v := strings.TrimSpace(*description)
		if v == "" || len(v) > 4000 {
			return ErrValidation("description must be non-empty and <= 4000 chars")
		}
		e.Description = v
	}
	if city != nil {
		v := strings.TrimSpace(*city)
		if v == "" || len(v) > 80 {
			return ErrValidation("city must be non-empty and <= 80 chars")
		}
		e.City = v
	}
	if category != nil {
		v := strings.TrimSpace(*category)
		if v == "" || len(v) > 80 {
			return ErrValidation("category must be non-empty and <= 80 chars")
		}
		e.Category = v
	}
	if start != nil {
		e.StartTime = start.UTC()
	}
	if end != nil {
		e.EndTime = end.UTC()
	}
	if (start != nil || end != nil) && !e.EndTime.After(e.StartTime) {
		return ErrValidation("end_time must be after start_time")
	}
	if capacity != nil {
		if *capacity < 0 {
			return ErrValidation("capacity must be >= 0 (0 means unlimited)")
		}
		e.Capacity = *capacity
	}
	e.UpdatedAt = now.UTC()
	return nil
}
