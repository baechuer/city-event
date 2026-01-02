// PATH: services/join-service/internal/infrastructure/postgres/outbox_worker.go
package postgres

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/pkg/logger"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	outboxBatchSize   = 20
	outboxMaxAttempts = 12 // ~ up to hours with exponential backoff
	confirmWait       = 300 * time.Millisecond
)

// backoff: exponential with jitter, bounded
func computeNextRetry(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// base: 2^attempt seconds, cap at 30 minutes
	sec := math.Pow(2, float64(attempt))
	if sec < 5 {
		sec = 5
	}
	if sec > 1800 {
		sec = 1800
	}

	d := time.Duration(sec) * time.Second

	// jitter +/-20%
	j := time.Duration(rand.Int63n(int64(d/5))) - d/10
	return d + j
}

func (r *Repository) StartOutboxWorker(ctx context.Context, rabbitURL, exchange string) {
	go func() {
		log := logger.Logger.With().Str("component", "outbox_worker").Logger()

		conn, err := amqp.Dial(rabbitURL)
		if err != nil {
			log.Error().Err(err).Msg("failed to connect rabbitmq for outbox publishing")
			return
		}
		defer conn.Close()

		ch, err := conn.Channel()
		if err != nil {
			log.Error().Err(err).Msg("failed to open rabbitmq channel for outbox publishing")
			return
		}
		defer ch.Close()

		if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
			log.Error().Err(err).Str("exchange", exchange).Msg("exchange declare failed")
			return
		}

		// Publisher confirms + mandatory returns
		if err := ch.Confirm(false); err != nil {
			log.Error().Err(err).Msg("publisher confirm enable failed")
			return
		}
		confirmCh := ch.NotifyPublish(make(chan amqp.Confirmation, 100))
		returnCh := ch.NotifyReturn(make(chan amqp.Return, 100))

		// Polling interval can be longer because next_retry_at gates load.
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var lastErr string
		var lastAt time.Time

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("stopped")
				return
			case <-ticker.C:
				if err := r.processOutboxBatch(ctx, ch, exchange, confirmCh, returnCh); err != nil {
					if err.Error() != lastErr || time.Since(lastAt) > 10*time.Second {
						log.Warn().Err(err).Msg("outbox batch failed")
						lastErr = err.Error()
						lastAt = time.Now()
					}
				} else {
					lastErr = ""
				}
			}
		}
	}()
}

func (r *Repository) processOutboxBatch(
	ctx context.Context,
	ch *amqp.Channel,
	exchange string,
	confirmCh <-chan amqp.Confirmation,
	returnCh <-chan amqp.Return,
) error {
	// Claim rows inside a tx so multiple workers don't double-publish.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	type msg struct {
		ID         uuid.UUID
		MessageID  uuid.UUID
		TraceID    string
		RoutingKey string
		Payload    []byte
		Attempt    int
	}

	rows, err := tx.Query(ctx, `
		SELECT id, message_id, trace_id, routing_key, payload, attempt
		FROM outbox
		WHERE status = 'pending'
		  AND next_retry_at <= NOW()
		ORDER BY next_retry_at ASC, occurred_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, outboxBatchSize)
	if err != nil {
		return err
	}
	defer rows.Close()

	var messages []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.ID, &m.MessageID, &m.TraceID, &m.RoutingKey, &m.Payload, &m.Attempt); err == nil {
			messages = append(messages, m)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// nothing to do
	if len(messages) == 0 {
		return tx.Commit(ctx)
	}

	// We commit the claim tx here to keep locks short.
	// Trade-off: "claimed" rows are still pending; a second worker could pick them up if next_retry_at is not moved.
	// So we "push next_retry_at slightly into future" to mark them in-flight.
	// This avoids long tx during network publish.
	inFlightUntil := time.Now().Add(15 * time.Second)
	for _, m := range messages {
		_, _ = tx.Exec(ctx, `
			UPDATE outbox
			SET next_retry_at = $2
			WHERE id = $1
		`, m.ID, inFlightUntil)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	log := logger.Logger.With().Str("component", "outbox_worker").Logger()

	for _, m := range messages {
		// Drain stale notifications
	DrainLoop:
		for {
			select {
			case <-returnCh:
				continue
			case <-confirmCh:
				continue
			default:
				break DrainLoop
			}
		}

		pub := amqp.Publishing{
			ContentType:   "application/json",
			Body:          m.Payload,
			DeliveryMode:  amqp.Persistent,
			Timestamp:     time.Now().UTC(),
			MessageId:     m.MessageID.String(),
			CorrelationId: m.TraceID,
			AppId:         "join-service",
		}

		// 1) transport publish
		if err := ch.PublishWithContext(ctx, exchange, m.RoutingKey, true, false, pub); err != nil {
			r.failOutbox(ctx, m, fmt.Sprintf("publish error: %v", err))
			continue
		}

		// 2) Wait for Confirm AND possible Return (mandatory)
		// Usually Return arrives BEFORE Confirm.
		var gotReturn bool
		var gotConfirm bool
		var conf amqp.Confirmation

		deadline := time.After(confirmWait * 2) // Give it enough time
	WaitLoop:
		for !gotConfirm {
			select {
			case ret := <-returnCh:
				gotReturn = true
				r.failOutbox(ctx, m, fmt.Sprintf("NO_ROUTE: code=%d text=%s exchange=%s rk=%s",
					ret.ReplyCode, ret.ReplyText, ret.Exchange, ret.RoutingKey))
			case c := <-confirmCh:
				gotConfirm = true
				conf = c
			case <-deadline:
				r.failOutbox(ctx, m, "confirm/return timeout")
				break WaitLoop
			}
		}

		if gotReturn {
			continue // Already called failOutbox
		}
		if !gotConfirm {
			continue // Timed out
		}

		if !conf.Ack {
			r.failOutbox(ctx, m, fmt.Sprintf("NACK: delivery_tag=%d", conf.DeliveryTag))
			continue
		}

		// success
		_, _ = r.pool.Exec(ctx, `
			UPDATE outbox
			SET status = 'sent',
			    last_error = NULL
			WHERE id = $1
		`, m.ID)

		log.Info().
			Str("outbox_id", m.ID.String()).
			Str("message_id", m.MessageID.String()).
			Str("routing_key", m.RoutingKey).
			Msg("published")
	}

	return nil
}

func (r *Repository) failOutbox(ctx context.Context, m struct {
	ID         uuid.UUID
	MessageID  uuid.UUID
	TraceID    string
	RoutingKey string
	Payload    []byte
	Attempt    int
}, errMsg string) {
	log := logger.Logger.With().Str("component", "outbox_worker").Logger()

	nextAttempt := m.Attempt + 1
	if nextAttempt >= outboxMaxAttempts {
		_, _ = r.pool.Exec(ctx, `
			UPDATE outbox
			SET status = 'dead',
			    attempt = $2,
			    last_error = $3
			WHERE id = $1
		`, m.ID, nextAttempt, errMsg)

		log.Error().
			Str("outbox_id", m.ID.String()).
			Str("message_id", m.MessageID.String()).
			Str("routing_key", m.RoutingKey).
			Int("attempt", nextAttempt).
			Msg("outbox moved to DEAD")
		return
	}

	delay := computeNextRetry(nextAttempt)
	_, _ = r.pool.Exec(ctx, `
		UPDATE outbox
		SET attempt = $2,
		    next_retry_at = NOW() + $3::interval,
		    last_error = $4
		WHERE id = $1
	`, m.ID, nextAttempt, fmt.Sprintf("%f seconds", delay.Seconds()), errMsg)

	log.Warn().
		Str("outbox_id", m.ID.String()).
		Str("message_id", m.MessageID.String()).
		Str("routing_key", m.RoutingKey).
		Int("attempt", nextAttempt).
		Dur("retry_in", delay).
		Msg("outbox publish failed; scheduled retry")
}
