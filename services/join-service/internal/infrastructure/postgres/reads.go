package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
)

func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// /me/joins : ORDER BY created_at DESC, id DESC
// cursor means "start after this item" in DESC order -> WHERE (created_at, id) < (cursor.created_at, cursor.id)
func (r *Repository) ListMyJoins(ctx context.Context, userID uuid.UUID, statuses []domain.JoinStatus, from, to *time.Time, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	limit = clampLimit(limit)
	args := []any{userID}
	where := "WHERE user_id = $1"

	argN := 2

	if len(statuses) > 0 {
		// build IN (...)
		ph := ""
		for i := range statuses {
			if i > 0 {
				ph += ","
			}
			ph += fmt.Sprintf("$%d", argN)
			args = append(args, string(statuses[i]))
			argN++
		}
		where += " AND status IN (" + ph + ")"
	}

	if from != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, *from)
		argN++
	}
	if to != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argN)
		args = append(args, *to)
		argN++
	}

	if cursor != nil {
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argN, argN+1)
		args = append(args, cursor.CreatedAt, cursor.ID)
		argN += 2
	}

	q := fmt.Sprintf(`
		SELECT id, event_id, user_id, status,
		       created_at, updated_at,
		       activated_at, canceled_at,
		       expired_at, expired_reason,
		       canceled_by, canceled_reason,
		       rejected_at, rejected_by, rejected_reason
		FROM joins
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT %d
	`, where, limit+1)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []domain.JoinRecord
	for rows.Next() {
		var rec domain.JoinRecord
		var status string
		if err := rows.Scan(
			&rec.ID, &rec.EventID, &rec.UserID, &status,
			&rec.CreatedAt, &rec.UpdatedAt,
			&rec.ActivatedAt, &rec.CanceledAt,
			&rec.ExpiredAt, &rec.ExpiredReason,
			&rec.CanceledBy, &rec.CanceledReason,
			&rec.RejectedAt, &rec.RejectedBy, &rec.RejectedReason,
		); err != nil {
			return nil, nil, err
		}
		rec.Status = domain.JoinStatus(status)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *domain.KeysetCursor
	if len(out) > limit {
		last := out[limit-1]
		next = &domain.KeysetCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}

// participants: active only, ORDER BY created_at ASC, id ASC
func (r *Repository) ListParticipants(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	return r.listByEventStatusASC(ctx, eventID, "active", limit, cursor)
}

// waitlist: waitlisted only, ORDER BY created_at ASC, id ASC
func (r *Repository) ListWaitlist(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	return r.listByEventStatusASC(ctx, eventID, "waitlisted", limit, cursor)
}

func (r *Repository) listByEventStatusASC(ctx context.Context, eventID uuid.UUID, status string, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	limit = clampLimit(limit)
	args := []any{eventID, status}
	where := "WHERE event_id = $1 AND status = $2"
	argN := 3

	// ASC cursor: WHERE (created_at, id) > (cursor.created_at, cursor.id)
	if cursor != nil {
		where += fmt.Sprintf(" AND (created_at, id) > ($%d, $%d)", argN, argN+1)
		args = append(args, cursor.CreatedAt, cursor.ID)
		argN += 2
	}

	q := fmt.Sprintf(`
		SELECT id, event_id, user_id, status,
		       created_at, updated_at,
		       activated_at, canceled_at
		FROM joins
		%s
		ORDER BY created_at ASC, id ASC
		LIMIT %d
	`, where, limit+1)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []domain.JoinRecord
	for rows.Next() {
		var rec domain.JoinRecord
		var st string
		if err := rows.Scan(
			&rec.ID, &rec.EventID, &rec.UserID, &st,
			&rec.CreatedAt, &rec.UpdatedAt,
			&rec.ActivatedAt, &rec.CanceledAt,
		); err != nil {
			return nil, nil, err
		}
		rec.Status = domain.JoinStatus(st)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *domain.KeysetCursor
	if len(out) > limit {
		last := out[limit-1]
		next = &domain.KeysetCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}

func (r *Repository) GetStats(ctx context.Context, eventID uuid.UUID) (domain.EventStats, error) {
	var s domain.EventStats
	s.EventID = eventID

	// Source of truth is your snapshot table
	err := r.pool.QueryRow(ctx, `
		SELECT capacity, active_count, waitlist_count, updated_at
		FROM event_capacity
		WHERE event_id = $1
	`, eventID).Scan(&s.Capacity, &s.ActiveCount, &s.WaitlistCount, &s.UpdatedAt)
	if err != nil {
		// keep semantics consistent with JoinEvent/CancelJoin
		return domain.EventStats{}, domain.ErrEventNotKnown
	}
	return s, nil
}

func (r *Repository) GetByEventAndUser(ctx context.Context, eventID, userID uuid.UUID) (domain.JoinRecord, error) {
	q := `
		SELECT id, event_id, user_id, status,
		       created_at, updated_at,
		       activated_at, canceled_at,
		       expired_at, expired_reason,
		       canceled_by, canceled_reason,
		       rejected_at, rejected_by, rejected_reason
		FROM joins
		WHERE event_id = $1 AND user_id = $2
		LIMIT 1
	`

	var rec domain.JoinRecord
	var status string
	err := r.pool.QueryRow(ctx, q, eventID, userID).Scan(
		&rec.ID, &rec.EventID, &rec.UserID, &status,
		&rec.CreatedAt, &rec.UpdatedAt,
		&rec.ActivatedAt, &rec.CanceledAt,
		&rec.ExpiredAt, &rec.ExpiredReason,
		&rec.CanceledBy, &rec.CanceledReason,
		&rec.RejectedAt, &rec.RejectedBy, &rec.RejectedReason,
	)
	if err != nil {
		// pgx.ErrNoRows -> logical not found
		// we map checking "participation" to "not joined" if no record
		return domain.JoinRecord{}, domain.ErrNotJoined
	}

	rec.Status = domain.JoinStatus(status)
	return rec, nil
}
