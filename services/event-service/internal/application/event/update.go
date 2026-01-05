package event

import (
	"context"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	zlog "github.com/rs/zerolog/log"
)

type UpdateCmd struct {
	ActorID   string
	ActorRole string
	EventID   string

	Title         *string
	Description   *string
	City          *string
	Category      *string
	StartTime     *time.Time
	EndTime       *time.Time
	Capacity      *int
	CoverImageIDs *[]string
}

func (s *Service) Update(ctx context.Context, cmd UpdateCmd) (*domain.Event, error) {
	ev, err := s.repo.GetByID(ctx, cmd.EventID)
	if err != nil {
		return nil, err
	}

	if !canManage(cmd.ActorID, cmd.ActorRole, ev.OwnerID) {
		return nil, domain.ErrForbidden("not allowed")
	}
	if ev.Status == domain.StatusCanceled {
		return nil, domain.ErrInvalidState("canceled event cannot be updated")
	}

	if err := ev.ApplyUpdate(cmd.Title, cmd.Description, cmd.City, cmd.Category, cmd.StartTime, cmd.EndTime, cmd.Capacity, cmd.CoverImageIDs, s.clock.Now()); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, ev); err != nil {
		return nil, err
	}

	// --- Cache Invalidation ---
	if s.cache != nil {
		key := cacheKeyEventDetails(ev.ID)
		if err := s.cache.Delete(ctx, key); err != nil {
			zlog.Warn().Err(err).Str("key", key).Msg("cache invalidate failed")
		}
	}

	return ev, nil
}
