package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// -------------------------
// Deadlock policy:
// Always lock in this order (for the same event_id):
//   1) event_capacity row (FOR UPDATE)
//   2) joins row for (event_id,user_id) if needed (FOR UPDATE)
//   3) optional waitlist row (FOR UPDATE SKIP LOCKED)
// This prevents cycles between JoinEvent/CancelJoin/Consumer(event.canceled).
// -------------------------

func (r *Repository) JoinEvent(ctx context.Context, traceID, idempotencyKey string, eventID, userID uuid.UUID) (domain.JoinStatus, error) {
	traceID = strings.TrimSpace(traceID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 0) Idempotency Check
	if idempotencyKey != "" {
		var insertedKey string
		err := tx.QueryRow(ctx, `
			INSERT INTO idempotency_keys (key, user_id, event_id, action, created_at, expires_at)
			VALUES ($1, $2, $3, 'join', NOW(), NOW() + INTERVAL '24 hours')
			ON CONFLICT (key) DO NOTHING
			RETURNING key
		`, idempotencyKey, userID, eventID).Scan(&insertedKey)

		if errors.Is(err, pgx.ErrNoRows) {
			// Key exists. Verify payload.
			var existUser, existEvent uuid.UUID
			var existAction string
			err := tx.QueryRow(ctx, `SELECT user_id, event_id, action FROM idempotency_keys WHERE key = $1`, idempotencyKey).Scan(&existUser, &existEvent, &existAction)
			if err != nil {
				return "", err
			}
			if existUser != userID || existEvent != eventID || existAction != "join" {
				return "", domain.ErrIdempotencyKeyMismatch
			}
			// Payload matches: Allow fall-through to see if we are already joined.
		} else if err != nil {
			return "", err
		}
	}

	// 1) Lock capacity FIRST (global lock for this event_id)
	var capacity, activeCount, waitlistCount int
	err = tx.QueryRow(ctx, `
		SELECT capacity, active_count, waitlist_count
		FROM event_capacity
		WHERE event_id = $1
		FOR UPDATE
	`, eventID).Scan(&capacity, &activeCount, &waitlistCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrEventNotKnown
		}
		return "", err
	}

	if capacity < 0 {
		return "", domain.ErrEventClosed
	}

	// 2) Ban check (same tx)
	var banned bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
		SELECT 1 FROM event_bans
		WHERE event_id = $1 AND user_id = $2
			AND (expires_at IS NULL OR expires_at > NOW())
		)
	`, eventID, userID).Scan(&banned)
	if err != nil {
		return "", err
	}
	if banned {
		return "", domain.ErrBanned
	}

	// 2.2) Lock (event_id,user_id) join row second
	var existing string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM joins
		WHERE event_id = $1 AND user_id = $2
		FOR UPDATE
	`, eventID, userID).Scan(&existing)

	if err == nil {
		// allow re-join only if previous is terminal
		if existing == string(domain.StatusActive) || existing == string(domain.StatusWaitlisted) {
			return "", domain.ErrAlreadyJoined
		}
		// else: canceled/expired/rejected -> reuse row
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	// 3) Decide status
	var newStatus domain.JoinStatus
	switch {
	case capacity == 0:
		newStatus = domain.StatusActive
	case activeCount < capacity:
		newStatus = domain.StatusActive
	default:
		if waitlistCount >= domain.WaitlistMax(capacity) {
			return "", domain.ErrEventFull
		}
		newStatus = domain.StatusWaitlisted
	}

	// 4) Insert or reuse join row
	if err == nil {
		// reuse existing row
		_, err = tx.Exec(ctx, `
			UPDATE joins
			SET status = $3,
				created_at = NOW(),
				updated_at = NOW(),
				activated_at = NULL,
				canceled_at = NULL,
				canceled_by = NULL,
				canceled_reason = NULL,
				rejected_at = NULL,
				rejected_by = NULL,
				rejected_reason = NULL,
				expired_at = NULL,
				expired_reason = NULL
			WHERE event_id = $1 AND user_id = $2
		`, eventID, userID, string(newStatus))
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO joins (event_id, user_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
		`, eventID, userID, string(newStatus))
	}
	if err != nil {
		return "", err
	}

	// 5) Counters (same tx, capacity row already locked)
	if newStatus == domain.StatusActive {
		_, _ = tx.Exec(ctx, `UPDATE event_capacity SET active_count = active_count + 1, updated_at = NOW() WHERE event_id = $1`, eventID)
	} else {
		_, _ = tx.Exec(ctx, `UPDATE event_capacity SET waitlist_count = waitlist_count + 1, updated_at = NOW() WHERE event_id = $1`, eventID)
	}

	// 6) Outbox (join.created)
	payload, _ := json.Marshal(map[string]any{
		"event_id": eventID,
		"user_id":  userID,
		"status":   newStatus,
	})
	_, _ = tx.Exec(ctx,
		`INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status) VALUES ($1, $2, $3, $4, NOW(), 'pending')`,
		uuid.New(), traceID, "join.created", payload,
	)

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return newStatus, nil
}

func (r *Repository) CancelJoin(ctx context.Context, traceID, idempotencyKey string, eventID, userID uuid.UUID) error {
	traceID = strings.TrimSpace(traceID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 0) Idempotency Check
	if idempotencyKey != "" {
		var insertedKey string
		err := tx.QueryRow(ctx, `
			INSERT INTO idempotency_keys (key, user_id, event_id, action, created_at, expires_at)
			VALUES ($1, $2, $3, 'cancel', NOW(), NOW() + INTERVAL '24 hours')
			ON CONFLICT (key) DO NOTHING
			RETURNING key
		`, idempotencyKey, userID, eventID).Scan(&insertedKey)

		if errors.Is(err, pgx.ErrNoRows) {
			// Key exists. Verify payload.
			var existUser, existEvent uuid.UUID
			var existAction string
			err := tx.QueryRow(ctx, `SELECT user_id, event_id, action FROM idempotency_keys WHERE key = $1`, idempotencyKey).Scan(&existUser, &existEvent, &existAction)
			if err != nil {
				return err
			}
			if existUser != userID || existEvent != eventID || existAction != "cancel" {
				return domain.ErrIdempotencyKeyMismatch
			}
			// Payload matches: Allow fall-through.
		} else if err != nil {
			return err
		}
	}

	// 1) Lock capacity FIRST
	var capacity, waitlistCount int
	err = tx.QueryRow(ctx, `
		SELECT capacity, waitlist_count
		FROM event_capacity
		WHERE event_id = $1
		FOR UPDATE
	`, eventID).Scan(&capacity, &waitlistCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrEventNotKnown
		}
		return err
	}

	// 2) Lock join row second
	var oldStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM joins
		WHERE event_id = $1 AND user_id = $2
		FOR UPDATE
	`, eventID, userID).Scan(&oldStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotJoined
		}
		return err
	}

	// idempotent cancel
	if oldStatus == string(domain.StatusCanceled) {
		return tx.Commit(ctx)
	}
	if oldStatus == string(domain.StatusExpired) || oldStatus == string(domain.StatusRejected) {
		// treat as not cancelable but idempotent no-op (product choice)
		return tx.Commit(ctx)
	}

	// 3) Mark canceled (keep row)
	_, err = tx.Exec(ctx, `
		UPDATE joins
		SET status = 'canceled',
		    canceled_at = NOW(),
		    canceled_by = $3,
		    canceled_reason = $4,
		    updated_at = NOW()
		WHERE event_id = $1 AND user_id = $2
	`, eventID, userID, userID, "self_cancel")
	if err != nil {
		return err
	}

	// 4) Counters + auto-promotion if freed slot
	if oldStatus == string(domain.StatusActive) {
		_, _ = tx.Exec(ctx, `UPDATE event_capacity SET active_count = active_count - 1, updated_at = NOW() WHERE event_id = $1`, eventID)

		if waitlistCount > 0 && capacity != 0 && capacity >= 0 {
			var promoUserID uuid.UUID
			err = tx.QueryRow(ctx, `
				SELECT user_id
				FROM joins
				WHERE event_id = $1 AND status = 'waitlisted'
				ORDER BY created_at ASC, id ASC
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			`, eventID).Scan(&promoUserID)

			if err == nil {
				_, _ = tx.Exec(ctx,
					`UPDATE joins
					 SET status = 'active', activated_at = NOW(), updated_at = NOW()
					 WHERE event_id = $1 AND user_id = $2`,
					eventID, promoUserID,
				)
				_, _ = tx.Exec(ctx, `
					UPDATE event_capacity
					SET active_count = active_count + 1,
					    waitlist_count = waitlist_count - 1,
					    updated_at = NOW()
					WHERE event_id = $1
				`, eventID)

				payload, _ := json.Marshal(map[string]any{
					"event_id": eventID,
					"user_id":  promoUserID,
					"reason":   "slot_freed",
				})
				_, _ = tx.Exec(ctx,
					`INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status)
					 VALUES ($1, $2, $3, $4, NOW(), 'pending')`,
					uuid.New(), traceID, "join.promoted", payload,
				)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
	} else if oldStatus == string(domain.StatusWaitlisted) {
		_, _ = tx.Exec(ctx, `UPDATE event_capacity SET waitlist_count = waitlist_count - 1, updated_at = NOW() WHERE event_id = $1`, eventID)
	}

	// 5) Outbox
	payload, _ := json.Marshal(map[string]any{
		"event_id":    eventID,
		"user_id":     userID,
		"prev_status": oldStatus,
	})
	_, _ = tx.Exec(ctx,
		`INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status)
		 VALUES ($1, $2, $3, $4, NOW(), 'pending')`,
		uuid.New(), traceID, "join.canceled", payload,
	)

	return tx.Commit(ctx)
}

func (r *Repository) InitCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO event_capacity (event_id, capacity, active_count, waitlist_count, created_at, updated_at)
		VALUES ($1, $2, 0, 0, NOW(), NOW())
		ON CONFLICT (event_id) DO UPDATE
		SET capacity = EXCLUDED.capacity,
		    updated_at = NOW()
	`, eventID, capacity)
	return err
}

// InitCapacityTx is used by the RabbitMQ snapshot consumer when it wants atomic tx with ProcessOnce.
func (r *Repository) InitCapacityTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, capacity int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO event_capacity (event_id, capacity, active_count, waitlist_count, created_at, updated_at)
		VALUES ($1, $2, 0, 0, NOW(), NOW())
		ON CONFLICT (event_id) DO UPDATE
		SET capacity = EXCLUDED.capacity,
		    updated_at = NOW()
	`, eventID, capacity)
	return err
}

// -------------------------
// event.canceled hard path (tx):
// - lock event_capacity
// - bulk update joins(active/waitlisted) -> expired with metadata
// - outbox per affected user to email-service
// - set counters to 0 and capacity=-1
// -------------------------

func (r *Repository) HandleEventCanceled(ctx context.Context, traceID string, eventID uuid.UUID, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.HandleEventCanceledTx(ctx, tx, traceID, eventID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// HandleEventCanceledTx is called from consumer inside ProcessOnce(...) transaction.
// IMPORTANT: do not call ProcessOnce here; caller already did it.
func (r *Repository) HandleEventCanceledTx(ctx context.Context, tx pgx.Tx, traceID string, eventID uuid.UUID, reason string) error {
	traceID = strings.TrimSpace(traceID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "event_canceled"
	}

	var capacity int
	err := tx.QueryRow(ctx, `SELECT capacity FROM event_capacity WHERE event_id = $1 FOR UPDATE`, eventID).Scan(&capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = tx.Exec(ctx, `INSERT INTO event_capacity (event_id, capacity, active_count, waitlist_count, created_at, updated_at) VALUES ($1, -1, 0, 0, NOW(), NOW()) ON CONFLICT (event_id) DO NOTHING`, eventID)
		err = tx.QueryRow(ctx, `SELECT capacity FROM event_capacity WHERE event_id = $1 FOR UPDATE`, eventID).Scan(&capacity)
	}
	if err != nil {
		return err
	}

	type affectedUser struct {
		UserID     uuid.UUID
		PrevStatus string
	}
	var users []affectedUser

	rows, err := tx.Query(ctx, `
		SELECT user_id, status 
		FROM joins 
		WHERE event_id = $1 AND status IN ('active', 'waitlisted') 
		FOR UPDATE`, eventID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var au affectedUser
		if err := rows.Scan(&au.UserID, &au.PrevStatus); err == nil {
			users = append(users, au)
		}
	}
	rows.Close()

	if len(users) > 0 {
		_, err = tx.Exec(ctx, `
			UPDATE joins 
			SET status = 'expired', expired_at = NOW(), expired_reason = $2, updated_at = NOW() 
			WHERE event_id = $1 AND status IN ('active', 'waitlisted')`,
			eventID, reason)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE event_capacity 
		SET capacity = -1, active_count = 0, waitlist_count = 0, updated_at = NOW() 
		WHERE event_id = $1`, eventID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, u := range users {
		payload, _ := json.Marshal(map[string]any{
			"event_id":     eventID.String(),
			"user_id":      u.UserID.String(),
			"prev_status":  u.PrevStatus,
			"reason":       reason,
			"occurred_at":  now.Format(time.RFC3339Nano),
			"trace_id":     traceID,
			"producer":     "join-service",
			"event_action": "canceled",
		})

		_, err = tx.Exec(ctx, `
			INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status) 
			VALUES ($1, $2, $3, $4, NOW(), 'pending')`,
			uuid.New(), traceID, "email.event_canceled", payload)
		if err != nil {
			return err
		}
	}

	return nil
}
