package postgres

import (
	"context"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TrackRepo struct {
	pool *pgxpool.Pool
}

func NewTrackRepo(pool *pgxpool.Pool) *TrackRepo {
	return &TrackRepo{pool: pool}
}

// InsertOutbox inserts a track event into the outbox table
func (r *TrackRepo) InsertOutbox(ctx context.Context, e domain.TrackEvent) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO track_outbox (actor_key, event_type, event_id, feed_type, position, request_id, bucket_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, e.ActorKey, e.EventType, e.EventID, e.FeedType, e.Position, e.RequestID, e.BucketDate)
	return err
}

// ProcessOutbox moves events from outbox to user_events
func (r *TrackRepo) ProcessOutbox(ctx context.Context, batchSize int) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Select unprocessed items
	rows, err := tx.Query(ctx, `
		SELECT id, actor_key, event_type, event_id, feed_type, position, request_id, bucket_date, created_at
		FROM track_outbox
		WHERE processed_at IS NULL
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var events []domain.TrackEvent
	var ids []interface{}
	for rows.Next() {
		var e domain.TrackEvent
		var id interface{}
		if err := rows.Scan(&id, &e.ActorKey, &e.EventType, &e.EventID, &e.FeedType, &e.Position, &e.RequestID, &e.BucketDate, &e.OccurredAt); err != nil {
			return 0, err
		}
		events = append(events, e)
		ids = append(ids, id)
	}

	if len(events) == 0 {
		return 0, nil
	}

	// Insert into user_events (with ON CONFLICT DO NOTHING for dedup)
	batch := &pgx.Batch{}
	for _, e := range events {
		batch.Queue(`
			INSERT INTO user_events (actor_key, event_type, event_id, feed_type, position, request_id, bucket_date, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (actor_key, event_id, event_type, bucket_date) DO NOTHING
		`, e.ActorKey, e.EventType, e.EventID, e.FeedType, e.Position, e.RequestID, e.BucketDate, e.OccurredAt)
	}

	br := tx.SendBatch(ctx, batch)
	for range events {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return 0, err
		}
	}
	br.Close()

	// Mark as processed
	_, err = tx.Exec(ctx, `
		UPDATE track_outbox SET processed_at = $1 WHERE id = ANY($2)
	`, time.Now(), ids)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return len(events), nil
}
