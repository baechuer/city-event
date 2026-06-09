package outboxrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultBatchSize = 100

type Publisher interface {
	Publish(context.Context, string, messaging.Envelope) error
}

type Relay struct {
	store     *Store
	publisher Publisher
	now       func() time.Time
	backoff   func(int) time.Duration
}

func NewRelay(store *Store, publisher Publisher) *Relay {
	return &Relay{
		store:     store,
		publisher: publisher,
		now:       time.Now,
		backoff:   retryBackoff,
	}
}

func (r *Relay) PublishBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = DefaultBatchSize
	}
	return r.store.withBatch(ctx, limit, r.now(), func(ctx context.Context, tx pgx.Tx, records []OutboxRecord) (int, error) {
		published := 0
		for _, record := range records {
			envelope, err := record.Envelope()
			if err != nil {
				if markErr := markFailed(ctx, tx, record.ID, record.Attempts+1, r.now().Add(r.backoff(record.Attempts+1)), err); markErr != nil {
					return published, markErr
				}
				return published, err
			}
			if err := r.publisher.Publish(ctx, record.RoutingKey, envelope); err != nil {
				if markErr := markFailed(ctx, tx, record.ID, record.Attempts+1, r.now().Add(r.backoff(record.Attempts+1)), err); markErr != nil {
					return published, markErr
				}
				return published, err
			}
			if err := markSent(ctx, tx, record.ID); err != nil {
				return published, err
			}
			published++
		}
		return published, nil
	})
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type OutboxRecord struct {
	ID            string
	AggregateType string
	AggregateID   string
	RoutingKey    string
	Payload       map[string]any
	CreatedAt     time.Time
	AvailableAt   time.Time
	Attempts      int
}

func (r OutboxRecord) Envelope() (messaging.Envelope, error) {
	correlationID := ""
	if value, ok := r.Payload["correlationId"].(string); ok {
		correlationID = value
	}
	return messaging.NewEnvelope(r.ID, r.RoutingKey, r.AggregateType, r.AggregateID, correlationID, r.CreatedAt, r.Payload)
}

type batchFn func(context.Context, pgx.Tx, []OutboxRecord) (int, error)

func (s *Store) withBatch(ctx context.Context, limit int, now time.Time, fn batchFn) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer rollback(ctx, tx)

	records, err := selectAvailable(ctx, tx, now, limit)
	if err != nil {
		return 0, err
	}
	published, err := fn(ctx, tx, records)
	if err != nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return published, errors.Join(err, commitErr)
		}
		return published, err
	}
	if err := tx.Commit(ctx); err != nil {
		return published, err
	}
	return published, nil
}

func selectAvailable(ctx context.Context, tx pgx.Tx, now time.Time, limit int) ([]OutboxRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, routing_key, payload, created_at, available_at, attempts
		FROM outbox_messages
		WHERE status IN ('PENDING', 'FAILED') AND available_at <= $1
		ORDER BY created_at ASC, id ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]OutboxRecord, 0)
	for rows.Next() {
		var record OutboxRecord
		var rawPayload []byte
		if err := rows.Scan(&record.ID, &record.AggregateType, &record.AggregateID, &record.RoutingKey, &rawPayload, &record.CreatedAt, &record.AvailableAt, &record.Attempts); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawPayload, &record.Payload); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func markSent(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE outbox_messages
		SET status = 'SENT', sent_at = now(), last_error = ''
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox row %s was not marked sent", id)
	}
	return nil
}

func markFailed(ctx context.Context, tx pgx.Tx, id string, attempts int, availableAt time.Time, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	tag, err := tx.Exec(ctx, `
		UPDATE outbox_messages
		SET status = 'FAILED', attempts = $2, available_at = $3, last_error = $4
		WHERE id = $1
	`, id, attempts, availableAt.UTC(), message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox row %s was not marked failed", id)
	}
	return nil
}

func retryBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Second
	}
	if attempts > 6 {
		attempts = 6
	}
	return time.Duration(attempts*attempts) * time.Second
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
