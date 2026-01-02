package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// IncrementParticipantCount increases the active_participants count for an event
func (r *Repo) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	query := `
		UPDATE events 
		SET active_participants = active_participants + 1,
		    updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to increment participant count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	return nil
}

// DecrementParticipantCount decreases the active_participants count for an event
func (r *Repo) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	query := `
		UPDATE events 
		SET active_participants = GREATEST(active_participants - 1, 0),
		    updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to decrement participant count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	return nil
}
