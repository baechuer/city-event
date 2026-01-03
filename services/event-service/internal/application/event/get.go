package event

import (
	"context"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	zlog "github.com/rs/zerolog/log"
)

func (s *Service) GetPublic(ctx context.Context, id string) (*domain.Event, error) {
	// 1. Try Cache
	key := cacheKeyEventDetails(id)
	var cached domain.Event

	if s.cache != nil {
		found, err := s.cache.Get(ctx, key, &cached)
		if err != nil {
			zlog.Warn().Err(err).Str("key", key).Msg("cache get failed")
		} else if found {
			zlog.Debug().Str("key", key).Msg("cache hit")
			return &cached, nil
		} else {
			zlog.Debug().Str("key", key).Msg("cache miss")
		}
	}

	// 2. DB Query
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Public: only published and not canceled; ended is still viewable (MVP)
	if e.Status != domain.StatusPublished {
		return nil, domain.ErrNotFound("event not found")
	}
	if e.Status == domain.StatusCanceled {
		return nil, domain.ErrNotFound("event not found")
	}

	// 3. Set Cache (Best Effort)
	if s.cache != nil {
		if err := s.cache.Set(ctx, key, e, s.ttlDetails); err != nil {
			zlog.Warn().Err(err).Str("key", key).Msg("cache set failed")
		}
	}

	return e, nil
}

func (s *Service) GetForOwner(ctx context.Context, id, actorID, actorRole string) (*domain.Event, error) {
	// No caching for owner/admin view (needs strict consistency)
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canEdit(e.OwnerID, actorID, actorRole) {
		return nil, domain.ErrForbidden("not allowed")
	}
	return e, nil
}

// GetBatch returns multiple events by their IDs (only published events).
// Used by BFF to avoid N+1 queries when enriching join records.
func (s *Service) GetBatch(ctx context.Context, ids []string) ([]*domain.Event, error) {
	return s.repo.GetByIDs(ctx, ids)
}
