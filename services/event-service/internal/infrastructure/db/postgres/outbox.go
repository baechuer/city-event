package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"math/rand"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
)

type txRepo struct {
	tx *sql.Tx
}

const insertOutboxSQL = `
INSERT INTO event_outbox (
  message_id, routing_key, body, created_at, status, next_retry_at
) VALUES ($1, $2, $3::jsonb, $4, 'pending', $4)
`

const selectEventForUpdateSQL = `
SELECT id, owner_id, title, description, city, category,
       start_time, end_time, capacity, active_participants, status,
       published_at, canceled_at, created_at, updated_at, cover_image_ids
FROM events WHERE id = $1
FOR UPDATE
`

func (r *txRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Event, error) {
	row := r.tx.QueryRowContext(ctx, selectEventForUpdateSQL, id)

	var e domain.Event
	var status string
	var coverIDsJSON string
	err := row.Scan(
		&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.City, &e.Category,
		&e.StartTime, &e.EndTime, &e.Capacity, &e.ActiveParticipants, &status,
		&e.PublishedAt, &e.CanceledAt, &e.CreatedAt, &e.UpdatedAt, &coverIDsJSON,
	)
	if err != nil {
		return nil, err
	}
	e.Status = domain.EventStatus(status)
	_ = json.Unmarshal([]byte(coverIDsJSON), &e.CoverImageIDs)
	return &e, nil
}

func (r *txRepo) Update(ctx context.Context, e *domain.Event) error {
	coverIDsJSON, _ := json.Marshal(e.CoverImageIDs)
	_, err := r.tx.ExecContext(ctx, updateEventSQL,
		e.ID,
		e.Title, e.Description, e.City, domain.NormalizeCity(e.City), e.Category,
		e.StartTime, e.EndTime, e.Capacity, string(e.Status),
		e.PublishedAt, e.CanceledAt, e.UpdatedAt, string(coverIDsJSON),
	)
	return err
}

func (r *txRepo) InsertOutbox(ctx context.Context, msg event.OutboxMessage) error {
	// Store JSON as text cast to jsonb for lib/pq compatibility.
	// We set next_retry_at = created_at so it is immediately eligible for polling.
	_, err := r.tx.ExecContext(ctx, insertOutboxSQL,
		msg.MessageID,
		msg.RoutingKey,
		string(msg.Body),
		msg.CreatedAt.UTC(),
	)
	return err
}

// --- outbox worker helpers (non-tx) ---

type outboxRow struct {
	ID         int64
	MessageID  string
	RoutingKey string
	Body       []byte
	Attempts   int
}

// Select pending messages that are due for retry.
// We use SKIP LOCKED to allow multiple workers.
const selectOutboxClaimsSQL = `
SELECT id, message_id, routing_key, body, attempts
FROM event_outbox
WHERE status = 'pending'
  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
ORDER BY next_retry_at ASC, created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED
`

const updateOutboxClaimSQL = `
UPDATE event_outbox
SET next_retry_at = $2,
    status = 'processing'
WHERE id = $1
`

const markOutboxSentSQL = `
UPDATE event_outbox
SET status = 'sent',
    sent_at = $2,
    last_error = NULL
WHERE id = $1
`

const markOutboxFailedSQL = `
UPDATE event_outbox
SET status = 'pending', -- retryable
    attempts = attempts + 1,
    next_retry_at = $2,
    last_error = $3
WHERE id = $1
`

const markOutboxDeadSQL = `
UPDATE event_outbox
SET status = 'dead',
    attempts = attempts + 1,
    last_error = $2
WHERE id = $1
`

const maxAttempts = 10

// StartOutboxWorker starts a polling worker that publishes pending outbox rows to RabbitMQ.
// Refactored to use Claim Check pattern:
// 1. Claim rows in DB TX (short)
// 2. Publish (Network, potentially slow)
// 3. Update status (DB TX, short)
func (r *Repo) StartOutboxWorker(ctx context.Context, pub event.EventPublisher) {
	go func() {
		// Jitter ticker to prevent thundering herd if multiple instances start together
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.processOutboxBatch(ctx, pub, 20); err != nil {
					// log error? using fmt for now as no logger injected
					// fmt.Printf("outbox error: %v\n", err)
				}
			}
		}
	}()
}

func (r *Repo) processOutboxBatch(ctx context.Context, pub event.EventPublisher, limit int) error {
	if limit <= 0 {
		limit = 50
	}

	// 1. Claim Phase
	// Use a short timeout for the claim transaction
	claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := r.db.BeginTx(claimCtx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(claimCtx, selectOutboxClaimsSQL, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	var batch []outboxRow
	for rows.Next() {
		var item outboxRow
		if err := rows.Scan(&item.ID, &item.MessageID, &item.RoutingKey, &item.Body, &item.Attempts); err != nil {
			return err
		}
		batch = append(batch, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(batch) == 0 {
		return tx.Commit() // nothing to do
	}

	// Mark as 'processing' and push retry into future (reservation) to prevent others from picking it up
	// if this worker crashes before reducing the timeout.
	reservation := time.Now().UTC().Add(30 * time.Second)
	for _, item := range batch {
		if _, err := tx.ExecContext(claimCtx, updateOutboxClaimSQL, item.ID, reservation); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// 2. Process Phase (No DB Lock)
	// We iterate through the batch. For each, we try to publish, then immediately mark result.
	// We can do this casually without holding a big lock.
	for _, item := range batch {
		r.processSingleItem(ctx, pub, item)
	}

	return nil
}

func (r *Repo) processSingleItem(ctx context.Context, pub event.EventPublisher, item outboxRow) {
	// Pub timeout
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := pub.PublishEvent(pubCtx, item.RoutingKey, item.MessageID, item.Body)

	// Result update timeout
	resCtx, cancelRes := context.WithTimeout(ctx, 3*time.Second)
	defer cancelRes()

	if err != nil {
		errMsg := err.Error()
		if item.Attempts >= maxAttempts {
			_, _ = r.db.ExecContext(resCtx, markOutboxDeadSQL, item.ID, errMsg)
		} else {
			// Exponential backoff
			backoff := time.Duration(math.Pow(2, float64(item.Attempts))) * time.Second
			// Add jitter
			backoff += time.Duration(rand.Intn(1000)) * time.Millisecond
			nextRetry := time.Now().UTC().Add(backoff)
			_, _ = r.db.ExecContext(resCtx, markOutboxFailedSQL, item.ID, nextRetry, errMsg)
		}
		return
	}

	// Success
	_, _ = r.db.ExecContext(resCtx, markOutboxSentSQL, item.ID, time.Now().UTC())
}
