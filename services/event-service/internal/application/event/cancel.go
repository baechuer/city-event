package event

import (
	"context"
	"encoding/json"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/google/uuid"
	zlog "github.com/rs/zerolog/log"
)

func (s *Service) Cancel(ctx context.Context, eventID, actorID, actorRole, reason string) (*domain.Event, error) {
	var out *domain.Event

	err := s.repo.WithTx(ctx, func(r TxEventRepo) error {
		ev, err := r.GetByIDForUpdate(ctx, eventID)
		if err != nil {
			return err
		}

		if !canManage(actorID, actorRole, ev.OwnerID) {
			return domain.ErrForbidden("not allowed")
		}

		switch ev.Status {
		case domain.StatusCanceled:
			return domain.ErrInvalidState("event already canceled")
		}

		now := s.clock.Now().UTC()

		ev.Status = domain.StatusCanceled
		ev.CanceledAt = &now
		ev.UpdatedAt = now

		if err := r.Update(ctx, ev); err != nil {
			return err
		}

		// --- Outbox (durable, at-least-once) ---
		messageID := uuid.NewString()
		env := DomainEventEnvelope[EventCanceledPayload]{
			Version:    EventVersion,
			Producer:   EventProducer,
			MessageID:  messageID,
			TraceID:    TraceIDFromContext(ctx),
			OccurredAt: now,
			Payload: EventCanceledPayload{
				EventID:   ev.ID,
				OwnerID:   ev.OwnerID,
				City:      ev.City,
				Category:  ev.Category,
				StartTime: ev.StartTime,
				EndTime:   ev.EndTime,
				Capacity:  ev.Capacity,
				Status:    string(ev.Status),
				Reason:    reason,
				ActorRole: actorRole,
			},
		}

		body, err := json.Marshal(env)
		if err != nil {
			return err
		}

		if err := r.InsertOutbox(ctx, OutboxMessage{
			MessageID:  messageID,
			RoutingKey: "event.canceled",
			Body:       body,
			CreatedAt:  now,
		}); err != nil {
			return err
		}

		out = ev
		return nil
	})
	if err != nil {
		return nil, err
	}

	// --- Cache Invalidation (best-effort, after commit) ---
	if s.cache != nil && out != nil {
		key := cacheKeyEventDetails(out.ID)
		if err := s.cache.Delete(ctx, key); err != nil {
			zlog.Warn().Err(err).Str("key", key).Msg("cache invalidate failed")
		}
	}

	return out, nil
}
