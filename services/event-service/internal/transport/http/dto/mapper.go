package dto

import (
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
)

func ToEventResp(e *domain.Event, now time.Time) EventResp {
	ended := e.IsEnded(now)

	// joinable rules for join-service to consume:
	// - must be published
	// - not ended
	// - not canceled
	joinable := (e.Status == domain.StatusPublished) && !ended && (e.Status != domain.StatusCanceled)

	return EventResp{
		ID:                 e.ID,
		OwnerID:            e.OwnerID,
		Title:              e.Title,
		Description:        e.Description,
		City:               e.City,
		Category:           e.Category,
		StartTime:          e.StartTime,
		EndTime:            e.EndTime,
		Capacity:           e.Capacity,
		ActiveParticipants: e.ActiveParticipants,
		Status:             string(e.Status),

		PublishedAt: e.PublishedAt,
		CanceledAt:  e.CanceledAt,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,

		Ended:    ended,
		Joinable: joinable,
	}
}
