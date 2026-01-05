package notify

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) EventCanceled(ctx context.Context, eventID, userID, reason, actorRole string) error {
	// 1. Idempotency Check
	// Key: email:sent:event_canceled:<eventID>:<userID>
	key := fmt.Sprintf("email:sent:event_canceled:%s:%s", eventID, userID)
	if s.idem != nil {
		seen, e := s.idem.Seen(ctx, key)
		if e != nil {
			return e // transient error from redis
		}
		if seen {
			s.lg.Info().
				Str("event_id", eventID).
				Str("user_id", userID).
				Msg("idempotent skip (already sent)")
			return nil
		}
	}

	// 2. Resolve User Email
	email, err := s.resolver.GetEmail(ctx, userID)
	if err != nil {
		// If we can't find email (404), maybe we should drop?
		// But for now, let's return error to retry in case it's network flake.
		// NOTE: You might want to distinguish 404 (non-retriable) vs 500.
		// For MVP, treating as error.
		return fmt.Errorf("resolve email failed: %w", err)
	}
	if email == "" {
		s.lg.Warn().Str("user_id", userID).Msg("user has no email; dropping")
		return nil
	}

	// 3. Send Email
	if err := s.sender.SendEventCanceled(ctx, email, eventID, reason); err != nil {
		return err
	}

	// 4. Mark Sent
	if s.idem != nil {
		// 7 days TTL for cancellation notices
		ttl := 7 * 24 * time.Hour
		if e := s.idem.MarkSent(ctx, key, ttl); e != nil {
			s.lg.Warn().Err(e).Str("key", key).Msg("idempotency mark failed (send already succeeded)")
			return nil
		}
	}

	s.lg.Info().
		Str("event_id", eventID).
		Str("user_id", userID).
		Str("email", email).
		Msg("event canceled email sent")
	return nil
}

func (s *Service) EventUnpublished(ctx context.Context, eventID, userID, reason, actorRole string) error {
	// 1. Idempotency Check
	key := fmt.Sprintf("email:sent:event_unpublished:%s:%s", eventID, userID)
	if s.idem != nil {
		seen, e := s.idem.Seen(ctx, key)
		if e != nil {
			return e
		}
		if seen {
			s.lg.Info().Str("event_id", eventID).Str("user_id", userID).Msg("idempotent skip")
			return nil
		}
	}

	// 2. Resolve User Email
	email, err := s.resolver.GetEmail(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve email failed: %w", err)
	}
	if email == "" {
		return nil
	}

	// 3. Send Email
	if err := s.sender.SendEventUnpublished(ctx, email, eventID, reason); err != nil {
		return err
	}

	// 4. Mark Sent
	if s.idem != nil {
		if e := s.idem.MarkSent(ctx, key, 7*24*time.Hour); e != nil {
			return nil
		}
	}

	return nil
}
