package media

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(context.Context, Asset) (Asset, error)
	Get(context.Context, string) (Asset, error)
	MarkUploaded(context.Context, string, string, time.Time) (Asset, error)
	ClaimNextUploaded(context.Context, time.Time) (Asset, bool, error)
	MarkReady(context.Context, string, time.Time) (Asset, error)
	MarkFailed(context.Context, string, string, time.Time) (Asset, error)
	CountByStatus(context.Context, string) (int, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, asset Asset) (Asset, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO media_assets (id, event_id, uploader_id, filename, content_type, size_bytes, bucket, object_key, status, failure_reason, created_at, updated_at, processed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, asset.ID, asset.EventID, asset.UploaderID, asset.Filename, asset.ContentType, asset.SizeBytes, asset.Bucket, asset.ObjectKey, asset.Status, asset.FailureReason, asset.CreatedAt, asset.UpdatedAt, asset.ProcessedAt)
	if err != nil {
		return Asset{}, err
	}
	return r.Get(ctx, asset.ID)
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Asset, error) {
	return scanAsset(r.pool.QueryRow(ctx, `
		SELECT id, event_id, uploader_id, filename, content_type, size_bytes, bucket, object_key, status, failure_reason, created_at, updated_at, processed_at
		FROM media_assets
		WHERE id = $1
	`, id))
}

func (r *PostgresRepository) MarkUploaded(ctx context.Context, id, userID string, now time.Time) (Asset, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Asset{}, err
	}
	defer rollback(ctx, tx)
	asset, err := scanAsset(tx.QueryRow(ctx, `
		SELECT id, event_id, uploader_id, filename, content_type, size_bytes, bucket, object_key, status, failure_reason, created_at, updated_at, processed_at
		FROM media_assets
		WHERE id = $1
		FOR UPDATE
	`, id))
	if err != nil {
		return Asset{}, err
	}
	if asset.UploaderID != userID {
		return Asset{}, ErrForbidden
	}
	if err := ValidateTransition(asset.Status, StatusUploaded); err != nil {
		return Asset{}, err
	}
	asset.Status = StatusUploaded
	asset.UpdatedAt = now.UTC()
	if _, err := tx.Exec(ctx, `UPDATE media_assets SET status = $2, updated_at = $3 WHERE id = $1`, asset.ID, asset.Status, asset.UpdatedAt); err != nil {
		return Asset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func (r *PostgresRepository) ClaimNextUploaded(ctx context.Context, now time.Time) (Asset, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Asset{}, false, err
	}
	defer rollback(ctx, tx)
	asset, err := scanAsset(tx.QueryRow(ctx, `
		SELECT id, event_id, uploader_id, filename, content_type, size_bytes, bucket, object_key, status, failure_reason, created_at, updated_at, processed_at
		FROM media_assets
		WHERE status = 'UPLOADED'
		ORDER BY updated_at ASC, id ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`))
	if errors.Is(err, ErrNotFound) {
		if err := tx.Commit(ctx); err != nil {
			return Asset{}, false, err
		}
		return Asset{}, false, nil
	}
	if err != nil {
		return Asset{}, false, err
	}
	if err := ValidateTransition(asset.Status, StatusProcessing); err != nil {
		return Asset{}, false, err
	}
	asset.Status = StatusProcessing
	asset.UpdatedAt = now.UTC()
	if _, err := tx.Exec(ctx, `UPDATE media_assets SET status = $2, updated_at = $3 WHERE id = $1`, asset.ID, asset.Status, asset.UpdatedAt); err != nil {
		return Asset{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Asset{}, false, err
	}
	return asset, true, nil
}

func (r *PostgresRepository) MarkReady(ctx context.Context, id string, now time.Time) (Asset, error) {
	return r.markFinal(ctx, id, StatusReady, "", now)
}

func (r *PostgresRepository) MarkFailed(ctx context.Context, id, reason string, now time.Time) (Asset, error) {
	return r.markFinal(ctx, id, StatusFailed, reason, now)
}

func (r *PostgresRepository) markFinal(ctx context.Context, id, status, reason string, now time.Time) (Asset, error) {
	processedAt := now.UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		SET status = $2, failure_reason = $3, updated_at = $4, processed_at = $4
		WHERE id = $1
	`, id, status, reason, processedAt)
	if err != nil {
		return Asset{}, err
	}
	return r.Get(ctx, id)
}

func (r *PostgresRepository) CountByStatus(ctx context.Context, status string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM media_assets WHERE status = $1`, status).Scan(&count)
	return count, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAsset(row scanner) (Asset, error) {
	var asset Asset
	err := row.Scan(&asset.ID, &asset.EventID, &asset.UploaderID, &asset.Filename, &asset.ContentType, &asset.SizeBytes, &asset.Bucket, &asset.ObjectKey, &asset.Status, &asset.FailureReason, &asset.CreatedAt, &asset.UpdatedAt, &asset.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	return asset, err
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

type MemoryRepository struct {
	mu     sync.Mutex
	assets map[string]Asset
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{assets: map[string]Asset{}}
}

func (r *MemoryRepository) Create(_ context.Context, asset Asset) (Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[asset.ID] = asset
	return asset, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	asset, ok := r.assets[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return asset, nil
}

func (r *MemoryRepository) MarkUploaded(_ context.Context, id, userID string, now time.Time) (Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	asset, ok := r.assets[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	if asset.UploaderID != userID {
		return Asset{}, ErrForbidden
	}
	if err := ValidateTransition(asset.Status, StatusUploaded); err != nil {
		return Asset{}, err
	}
	asset.Status = StatusUploaded
	asset.UpdatedAt = now.UTC()
	r.assets[id] = asset
	return asset, nil
}

func (r *MemoryRepository) ClaimNextUploaded(_ context.Context, now time.Time) (Asset, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, asset := range r.assets {
		if asset.Status == StatusUploaded {
			asset.Status = StatusProcessing
			asset.UpdatedAt = now.UTC()
			r.assets[id] = asset
			return asset, true, nil
		}
	}
	return Asset{}, false, nil
}

func (r *MemoryRepository) MarkReady(_ context.Context, id string, now time.Time) (Asset, error) {
	return r.mark(id, StatusReady, "", now)
}

func (r *MemoryRepository) MarkFailed(_ context.Context, id, reason string, now time.Time) (Asset, error) {
	return r.mark(id, StatusFailed, reason, now)
}

func (r *MemoryRepository) mark(id, status, reason string, now time.Time) (Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	asset, ok := r.assets[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	processedAt := now.UTC()
	asset.Status = status
	asset.FailureReason = reason
	asset.UpdatedAt = processedAt
	asset.ProcessedAt = &processedAt
	r.assets[id] = asset
	return asset, nil
}

func (r *MemoryRepository) CountByStatus(_ context.Context, status string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, asset := range r.assets {
		if asset.Status == status {
			count++
		}
	}
	return count, nil
}
