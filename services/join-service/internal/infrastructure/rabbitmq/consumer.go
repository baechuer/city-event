package rabbitmq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/contracts/event"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

const (
	supportedVersion = 1

	rkEventPublished = "event.published"
	rkEventUpdated   = "event.updated"
	rkEventCanceled  = "event.canceled"
)

type Consumer struct {
	rabbitURL string
	exchange  string
	repo      domain.JoinRepository
}

func NewConsumer(rabbitURL, exchange string, repo domain.JoinRepository) *Consumer {
	return &Consumer{
		rabbitURL: strings.TrimSpace(rabbitURL),
		exchange:  strings.TrimSpace(exchange),
		repo:      repo,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	log := logger.Logger.With().Str("component", "rabbitmq_consumer").Logger()

	conn, err := amqp.Dial(c.rabbitURL)
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}

	// Ensure exchange exists (idempotent)
	if err := ch.ExchangeDeclare(c.exchange, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	// Declare Dead Letter Exchange and Queue for failed messages
	dlxName := c.exchange + ".dlx"
	dlqName := "join-service.event-snapshots.dlq"

	if err := ch.ExchangeDeclare(dlxName, "fanout", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	if _, err := ch.QueueDeclare(dlqName, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	if err := ch.QueueBind(dlqName, "", dlxName, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	// Main queue with DLX configuration
	q, err := ch.QueueDeclare(
		"join-service.event-snapshots",
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		amqp.Table{
			"x-dead-letter-exchange": dlxName,
		},
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	for _, rk := range []string{rkEventPublished, rkEventUpdated, rkEventCanceled} {
		if err := ch.QueueBind(q.Name, rk, c.exchange, false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return err
		}
	}

	if err := ch.Qos(10, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	deliveries, err := ch.Consume(q.Name, "join-service", false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	go func() {
		defer func() {
			_ = ch.Close()
			_ = conn.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}

				if err := c.handleDelivery(ctx, d); err != nil {
					// Nack with requeue=false sends to DLQ (via x-dead-letter-exchange)
					_ = d.Nack(false, false)
					continue
				}
				_ = d.Ack(false)
			}
		}
	}()

	log.Info().Str("queue", q.Name).Msg("consumer started")
	return nil
}

func (c *Consumer) handleDelivery(ctx context.Context, d amqp.Delivery) error {
	baseLog := logger.Logger.With().
		Str("component", "rabbitmq_consumer").
		Str("routing_key", d.RoutingKey).
		Logger()

	var env event.DomainEventEnvelope[json.RawMessage]
	if err := json.Unmarshal(d.Body, &env); err != nil {
		baseLog.Warn().Err(err).Msg("invalid envelope json; dropping")
		return nil // poison => drop
	}

	if env.Version != supportedVersion {
		baseLog.Warn().Int("version", env.Version).Msg("unsupported envelope version; dropping")
		return nil
	}

	// message_id: prefer envelope.message_id, then AMQP MessageId, else hash fallback
	msgID := strings.TrimSpace(env.MessageID)
	if msgID == "" {
		msgID = strings.TrimSpace(d.MessageId)
	}
	if msgID == "" {
		h := sha256.Sum256(append([]byte(d.RoutingKey+"\n"), d.Body...))
		msgID = "hash:" + hex.EncodeToString(h[:])
	}

	log := baseLog.With().
		Str("message_id", msgID).
		Str("trace_id", strings.TrimSpace(env.TraceID)).
		Logger()

	// Strong path: atomic "dedupe fence + side effects" in the SAME DB tx
	type inboxTx interface {
		ProcessOnce(ctx context.Context, messageID, handlerName string, fn func(tx pgx.Tx) error) (bool, error)
		InitCapacityTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, capacity int) error
	}
	const handlerName = "event_snapshots"

	if r, ok := any(c.repo).(inboxTx); ok {
		processed, err := r.ProcessOnce(ctx, msgID, handlerName, func(tx pgx.Tx) error {
			return applySnapshotTx(ctx, r, tx, d.RoutingKey, env.Payload, strings.TrimSpace(env.TraceID), log)
		})
		if err != nil {
			log.Error().Err(err).Msg("processing failed (requeue)")
			return err
		}
		if !processed {
			log.Info().Msg("duplicate delivery ignored")
		}
		return nil
	}

	// Compatibility path: optional dedupe (non-atomic)
	type processedMarker interface {
		TryMarkProcessed(ctx context.Context, messageID, handlerName string) (bool, error)
	}

	if pm, ok := any(c.repo).(processedMarker); ok {
		first, err := pm.TryMarkProcessed(ctx, msgID, handlerName)
		if err != nil {
			log.Error().Err(err).Msg("processed_messages insert failed (requeue)")
			return err
		}
		if !first {
			log.Info().Msg("duplicate delivery ignored")
			return nil
		}
	} else {
		// No dedupe available -> still process; better than dropping.
		log.Warn().Msg("repo does not support processed_messages; processing without dedupe")
	}

	return applySnapshot(ctx, c.repo, d.RoutingKey, env.Payload, log)
}

func applySnapshot(ctx context.Context, repo domain.JoinRepository, routingKey string, raw json.RawMessage, log zerolog.Logger) error {
	switch routingKey {
	case rkEventPublished, rkEventUpdated:
		var p event.EventPublishedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Warn().Err(err).Msg("invalid payload json; dropping")
			return nil
		}
		if strings.TrimSpace(p.EventID) == "" || p.Capacity == nil {
			log.Warn().Msg("missing fields; dropping")
			return nil
		}
		eid, err := uuid.Parse(p.EventID)
		if err != nil {
			log.Warn().Err(err).Msg("invalid event_id; dropping")
			return nil
		}
		return repo.InitCapacity(ctx, eid, *p.Capacity)

	case rkEventCanceled:
		var p event.EventCanceledPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Warn().Err(err).Msg("invalid payload json; dropping")
			return nil
		}
		if strings.TrimSpace(p.EventID) == "" {
			log.Warn().Msg("missing event_id; dropping")
			return nil
		}
		eid, err := uuid.Parse(p.EventID)
		if err != nil {
			log.Warn().Err(err).Msg("invalid event_id; dropping")
			return nil
		}
		return repo.InitCapacity(ctx, eid, -1)

	default:
		log.Warn().Msg("unknown routing key; ignoring")
		return nil
	}
}

func applySnapshotTx(
	ctx context.Context,
	r interface {
		InitCapacityTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, capacity int) error
	},
	tx pgx.Tx,
	routingKey string,
	raw json.RawMessage,
	traceID string,
	log zerolog.Logger,
) error {
	switch routingKey {
	case rkEventPublished, rkEventUpdated:
		var p event.EventPublishedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Warn().Err(err).Msg("invalid payload json; dropping")
			return nil
		}
		if strings.TrimSpace(p.EventID) == "" || p.Capacity == nil {
			log.Warn().Msg("missing fields; dropping")
			return nil
		}
		eid, err := uuid.Parse(p.EventID)
		if err != nil {
			log.Warn().Err(err).Msg("invalid event_id; dropping")
			return nil
		}
		return r.InitCapacityTx(ctx, tx, eid, *p.Capacity)

	case rkEventCanceled:
		var p event.EventCanceledPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Warn().Err(err).Msg("invalid payload json; dropping")
			return nil
		}

		// tolerate legacy field
		eidStr := strings.TrimSpace(p.EventID)
		if eidStr == "" {
			eidStr = strings.TrimSpace(p.ID)
		}
		if eidStr == "" {
			log.Warn().Msg("missing event_id; dropping")
			return nil
		}
		eid, err := uuid.Parse(eidStr)
		if err != nil {
			log.Warn().Err(err).Msg("invalid event_id; dropping")
			return nil
		}

		reason := strings.TrimSpace(p.Reason)
		if reason == "" {
			reason = "event_canceled"
		}

		// HARD PATH: bulk expire + outbox, inside the SAME ProcessOnce tx
		type canceledHandler interface {
			HandleEventCanceledTx(ctx context.Context, tx pgx.Tx, traceID string, eventID uuid.UUID, reason string) error
		}
		if h, ok := any(r).(canceledHandler); ok {
			// trace id comes from envelope (already logged in caller)
			// caller already validated env, but we don't have env here; pass empty or let repo default.
			// Better: thread trace_id into applySnapshotTx signature; but keep minimal diff:
			return h.HandleEventCanceledTx(ctx, tx, traceID, eid, reason)
		}

		// Fallback: at least close snapshot
		return r.InitCapacityTx(ctx, tx, eid, -1)

	default:
		log.Warn().Msg("unknown routing key; ignoring")
		return nil
	}
}
