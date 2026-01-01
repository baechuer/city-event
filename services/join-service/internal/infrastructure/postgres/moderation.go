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
)

func (r *Repository) Kick(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string) error {
	traceID = strings.TrimSpace(traceID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "kicked"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// lock capacity first
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

	// lock join row
	var oldStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM joins
		WHERE event_id = $1 AND user_id = $2
		FOR UPDATE
	`, eventID, targetUserID).Scan(&oldStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotJoined
		}
		return err
	}

	// if already terminal, idempotent
	if oldStatus == string(domain.StatusRejected) || oldStatus == string(domain.StatusExpired) {
		return tx.Commit(ctx)
	}

	// mark rejected (moderation)
	_, err = tx.Exec(ctx, `
		UPDATE joins
		SET status = 'rejected',
		    rejected_at = NOW(),
		    rejected_by = $3,
		    rejected_reason = $4,
		    updated_at = NOW()
		WHERE event_id = $1 AND user_id = $2
	`, eventID, targetUserID, actorID, reason)
	if err != nil {
		return err
	}

	// counters + promotion
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
					`UPDATE joins SET status='active', activated_at=NOW(), updated_at=NOW()
					 WHERE event_id=$1 AND user_id=$2`,
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
					 VALUES ($1,$2,$3,$4,NOW(),'pending')`,
					uuid.New(), traceID, "join.promoted", payload,
				)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
	} else if oldStatus == string(domain.StatusWaitlisted) {
		_, _ = tx.Exec(ctx, `UPDATE event_capacity SET waitlist_count = waitlist_count - 1, updated_at = NOW() WHERE event_id = $1`, eventID)
	}

	payload, _ := json.Marshal(map[string]any{
		"event_id":    eventID,
		"user_id":     targetUserID,
		"actor_id":    actorID,
		"prev_status": oldStatus,
		"reason":      reason,
		"action":      "kicked",
	})
	_, _ = tx.Exec(ctx,
		`INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status)
		 VALUES ($1,$2,$3,$4,NOW(),'pending')`,
		uuid.New(), traceID, "join.kicked", payload,
	)

	return tx.Commit(ctx)
}

func (r *Repository) Ban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string, expiresAt *time.Time) error {
	traceID = strings.TrimSpace(traceID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "banned"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// upsert ban row
	_, err = tx.Exec(ctx, `
		INSERT INTO event_bans (event_id, user_id, actor_id, reason, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,NOW())
		ON CONFLICT (event_id, user_id) DO UPDATE
		  SET actor_id = EXCLUDED.actor_id,
		      reason = EXCLUDED.reason,
		      expires_at = EXCLUDED.expires_at,
		      created_at = NOW()
	`, eventID, targetUserID, actorID, reason, expiresAt)
	if err != nil {
		return err
	}

	// if user currently active/waitlisted -> kick them (same tx) so ban takes effect immediately
	// reuse Kick logic but inline minimal (to avoid nested tx)
	var oldStatus string
	err = tx.QueryRow(ctx, `
		SELECT status FROM joins
		WHERE event_id=$1 AND user_id=$2
		FOR UPDATE
	`, eventID, targetUserID).Scan(&oldStatus)
	if err == nil && (oldStatus == string(domain.StatusActive) || oldStatus == string(domain.StatusWaitlisted)) {
		// do a “kick” effect: rejected
		_, _ = tx.Exec(ctx, `
			UPDATE joins
			SET status='rejected',
			    rejected_at=NOW(),
			    rejected_by=$3,
			    rejected_reason=$4,
			    updated_at=NOW()
			WHERE event_id=$1 AND user_id=$2
		`, eventID, targetUserID, actorID, "banned:"+reason)

		// counters/promotion: simplest is call a small helper; but keep short:
		// lock capacity row and adjust counts
		var capacity, waitlistCount int
		if err2 := tx.QueryRow(ctx, `
			SELECT capacity, waitlist_count FROM event_capacity WHERE event_id=$1 FOR UPDATE
		`, eventID).Scan(&capacity, &waitlistCount); err2 == nil {

			if oldStatus == string(domain.StatusActive) {
				_, _ = tx.Exec(ctx, `UPDATE event_capacity SET active_count=active_count-1, updated_at=NOW() WHERE event_id=$1`, eventID)
				if waitlistCount > 0 && capacity != 0 && capacity >= 0 {
					var promoUserID uuid.UUID
					err3 := tx.QueryRow(ctx, `
						SELECT user_id FROM joins
						WHERE event_id=$1 AND status='waitlisted'
						ORDER BY created_at ASC, id ASC
						LIMIT 1
						FOR UPDATE SKIP LOCKED
					`, eventID).Scan(&promoUserID)
					if err3 == nil {
						_, _ = tx.Exec(ctx, `UPDATE joins SET status='active', activated_at=NOW(), updated_at=NOW() WHERE event_id=$1 AND user_id=$2`, eventID, promoUserID)
						_, _ = tx.Exec(ctx, `UPDATE event_capacity SET active_count=active_count+1, waitlist_count=waitlist_count-1, updated_at=NOW() WHERE event_id=$1`, eventID)
					}
				}
			} else if oldStatus == string(domain.StatusWaitlisted) {
				_, _ = tx.Exec(ctx, `UPDATE event_capacity SET waitlist_count=waitlist_count-1, updated_at=NOW() WHERE event_id=$1`, eventID)
			}
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"event_id": eventID,
		"user_id":  targetUserID,
		"actor_id": actorID,
		"reason":   reason,
		"action":   "banned",
	})
	_, _ = tx.Exec(ctx, `
		INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status)
		VALUES ($1,$2,$3,$4,NOW(),'pending')
	`, uuid.New(), traceID, "join.banned", payload)

	return tx.Commit(ctx)
}

func (r *Repository) Unban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID) error {
	traceID = strings.TrimSpace(traceID)

	_, err := r.pool.Exec(ctx, `DELETE FROM event_bans WHERE event_id=$1 AND user_id=$2`, eventID, targetUserID)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"event_id": eventID,
		"user_id":  targetUserID,
		"actor_id": actorID,
		"action":   "unbanned",
	})
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO outbox (message_id, trace_id, routing_key, payload, occurred_at, status)
		VALUES ($1,$2,$3,$4,NOW(),'pending')
	`, uuid.New(), traceID, "join.unbanned", payload)

	return nil
}
