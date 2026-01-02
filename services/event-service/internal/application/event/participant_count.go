package event

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// IncrementParticipantCount increases the active participant count for an event
func (s *Service) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	// Invalidate cache for this event
	if s.cache != nil {
		cacheKey := fmt.Sprintf("event:%s", eventID.String())
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			log.Warn().Err(err).Str("event_id", eventID.String()).Msg("failed to invalidate cache")
		}
	}

	// Update in database
	err := s.repo.IncrementParticipantCount(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to increment participant count: %w", err)
	}

	log.Info().Str("event_id", eventID.String()).Msg("participant count incremented")
	return nil
}

// DecrementParticipantCount decreases the active participant count for an event
func (s *Service) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	// Invalidate cache for this event
	if s.cache != nil {
		cacheKey := fmt.Sprintf("event:%s", eventID.String())
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			log.Warn().Err(err).Str("event_id", eventID.String()).Msg("failed to invalidate cache")
		}
	}

	// Update in database
	err := s.repo.DecrementParticipantCount(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to decrement participant count: %w", err)
	}

	log.Info().Str("event_id", eventID.String()).Msg("participant count decremented")
	return nil
}
