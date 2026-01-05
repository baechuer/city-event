package event

import (
	"context"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
)

type CreateCmd struct {
	ActorID   string
	ActorRole string

	Title         string
	Description   string
	City          string
	Category      string
	StartTime     time.Time
	EndTime       time.Time
	Capacity      int
	CoverImageIDs []string
}

func (s *Service) Create(ctx context.Context, cmd CreateCmd) (*domain.Event, error) {
	if !canCreate(cmd.ActorRole) {
		return nil, domain.ErrForbidden("only organizer/admin can create events")
	}
	now := s.clock.Now()
	e, err := domain.NewDraft(cmd.ActorID, cmd.Title, cmd.Description, cmd.City, cmd.Category, cmd.StartTime, cmd.EndTime, cmd.Capacity, cmd.CoverImageIDs, now)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}
