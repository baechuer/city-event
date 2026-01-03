package event

import (
	"context"
	"encoding/json"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/google/uuid"
	zlog "github.com/rs/zerolog/log"
)

func (s *Service) Unpublish(ctx context.Context, eventID, actorID, actorRole string) (*domain.Event, error) {
	var out *domain.Event

	err := s.repo.WithTx(ctx, func(r TxEventRepo) error {
		ev, err := r.GetByIDForUpdate(ctx, eventID)
		if err != nil {
			return err
		}

		if !canManage(actorID, actorRole, ev.OwnerID) {
			return domain.ErrForbidden("not allowed")
		}

		// Validation: Must be published to unpublish
		if ev.Status != domain.StatusPublished {
			return domain.ErrInvalidState("event must be published to unpublish")
		}

		now := s.clock.Now().UTC()

		ev.Status = domain.StatusDraft
		ev.UpdatedAt = now
		// Do not set CanceledAt (Unpublish != Cancel)

		if err := r.Update(ctx, ev); err != nil {
			return err
		}

		// --- Outbox ---
		messageID := uuid.NewString()
		// Re-use CanceledPayload or define UnpublishedPayload?
		// Usually Unpublished is similar to Canceled (removed from feed).
		// For MVP, generic Envelope is fine.
		// Or "EventUnpublishedPayload".
		// I'll stick to generic payload map or struct if available.
		// cancel.go used `EventCanceledPayload`.
		// I should check `ports.go` for defined payloads.
		// Assuming I can use a generic map for now or `EventUnpublished` if defined.
		// To be safe, I'll check ports.go first to avoid compilation error.
		// But for now I'll use a local struct or map.

		// I'll assume simple map for payload to match cleanliness or reuse struct.
		// `cancel.go` used `EventCanceledPayload`.
		// I will assume `EventUnpublishedPayload` does NOT exist yet.
		// I will use `map[string]any`.

		payload := map[string]any{
			"event_id": ev.ID,
			"owner_id": ev.OwnerID,
			"status":   ev.Status,
		}

		env := DomainEventEnvelope[map[string]any]{
			Version:    EventVersion,
			Producer:   EventProducer,
			MessageID:  messageID,
			TraceID:    TraceIDFromContext(ctx),
			OccurredAt: now,
			Payload:    payload,
		}
		// Wait, Envelope definition in cancel.go seems to infer Type from payload? or Routing Key?
		// cancel.go: `RoutingKey: "event.canceled"`.

		body, err := json.Marshal(env)
		if err != nil {
			return err
		}

		if err := r.InsertOutbox(ctx, OutboxMessage{
			MessageID:  messageID,
			RoutingKey: "event.unpublished",
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

	// --- Cache Invalidation ---
	if s.cache != nil && out != nil {
		key := cacheKeyEventDetails(out.ID)
		if err := s.cache.Delete(ctx, key); err != nil {
			zlog.Warn().Err(err).Str("key", key).Msg("cache invalidate failed")
		}
	}

	return out, nil
}
