package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/baechuer/cityevents/services/media-service/internal/domain"
)

// UploadRepository handles database operations for uploads.
type UploadRepository struct {
	pool *pgxpool.Pool
}

// NewUploadRepository creates a new upload repository.
func NewUploadRepository(pool *pgxpool.Pool) *UploadRepository {
	return &UploadRepository{pool: pool}
}

// Create inserts a new upload record.
func (r *UploadRepository) Create(ctx context.Context, upload *domain.Upload) error {
	derivedJSON, err := json.Marshal(upload.DerivedKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal derived_keys: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO media_uploads (id, owner_id, purpose, status, raw_object_key, derived_keys, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, upload.ID, upload.OwnerID, upload.Purpose, upload.Status, upload.RawObjectKey, derivedJSON, upload.ErrorMessage, upload.CreatedAt, upload.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create upload: %w", err)
	}
	return nil
}

// GetByID retrieves an upload by ID.
func (r *UploadRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Upload, error) {
	var u domain.Upload
	var derivedJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, purpose, status, raw_object_key, derived_keys, error_message, created_at, updated_at
		FROM media_uploads WHERE id = $1
	`, id).Scan(&u.ID, &u.OwnerID, &u.Purpose, &u.Status, &u.RawObjectKey, &derivedJSON, &u.ErrorMessage, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get upload: %w", err)
	}

	if len(derivedJSON) > 0 {
		if err := json.Unmarshal(derivedJSON, &u.DerivedKeys); err != nil {
			return nil, fmt.Errorf("failed to unmarshal derived_keys: %w", err)
		}
	}
	return &u, nil
}

// UpdateStatus updates the status of an upload.
func (r *UploadRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UploadStatus) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE media_uploads SET status = $2, updated_at = $3 WHERE id = $1
	`, id, status, time.Now())
	return err
}

// UpdateStatusWithError updates status and error message.
func (r *UploadRepository) UpdateStatusWithError(ctx context.Context, id uuid.UUID, status domain.UploadStatus, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE media_uploads SET status = $2, error_message = $3, updated_at = $4 WHERE id = $1
	`, id, status, errMsg, time.Now())
	return err
}

// UpdateDerivedKeys updates the derived keys after processing.
func (r *UploadRepository) UpdateDerivedKeys(ctx context.Context, id uuid.UUID, derivedKeys map[string]string) error {
	derivedJSON, err := json.Marshal(derivedKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal derived_keys: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE media_uploads SET derived_keys = $2, status = $3, updated_at = $4 WHERE id = $1
	`, id, derivedJSON, domain.StatusReady, time.Now())
	return err
}

// GetByOwnerAndPurpose retrieves the latest upload for an owner with a specific purpose.
func (r *UploadRepository) GetByOwnerAndPurpose(ctx context.Context, ownerID uuid.UUID, purpose domain.UploadPurpose) (*domain.Upload, error) {
	var u domain.Upload
	var derivedJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, purpose, status, raw_object_key, derived_keys, error_message, created_at, updated_at
		FROM media_uploads 
		WHERE owner_id = $1 AND purpose = $2 AND status = $3
		ORDER BY created_at DESC
		LIMIT 1
	`, ownerID, purpose, domain.StatusReady).Scan(&u.ID, &u.OwnerID, &u.Purpose, &u.Status, &u.RawObjectKey, &derivedJSON, &u.ErrorMessage, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get upload: %w", err)
	}

	if len(derivedJSON) > 0 {
		if err := json.Unmarshal(derivedJSON, &u.DerivedKeys); err != nil {
			return nil, fmt.Errorf("failed to unmarshal derived_keys: %w", err)
		}
	}
	return &u, nil
}

// ListStale retrieves uploads that are stale (pending/failed for too long).
func (r *UploadRepository) ListStale(ctx context.Context, pendingAge, failedAge time.Duration, limit int) ([]*domain.Upload, error) {
	cutoffPending := time.Now().Add(-pendingAge)
	cutoffFailed := time.Now().Add(-failedAge)

	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, purpose, status, raw_object_key, derived_keys, error_message, created_at, updated_at
		FROM media_uploads 
		WHERE (status = $1 AND created_at < $2) 
		   OR (status = $3 AND created_at < $4)
		LIMIT $5
	`, domain.StatusPending, cutoffPending, domain.StatusFailed, cutoffFailed, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list stale uploads: %w", err)
	}
	defer rows.Close()

	var uploads []*domain.Upload
	for rows.Next() {
		var u domain.Upload
		var derivedJSON []byte

		if err := rows.Scan(&u.ID, &u.OwnerID, &u.Purpose, &u.Status, &u.RawObjectKey, &derivedJSON, &u.ErrorMessage, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan upload: %w", err)
		}

		if len(derivedJSON) > 0 {
			_ = json.Unmarshal(derivedJSON, &u.DerivedKeys)
		}
		uploads = append(uploads, &u)
	}

	return uploads, nil
}

// Delete permanently removes an upload record.
func (r *UploadRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM media_uploads WHERE id = $1", id)
	return err
}
