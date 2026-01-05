package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/baechuer/cityevents/services/media-service/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadRepository_Stale(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping integration test: DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping integration test: database not reachable: %v", err)
	}

	repo := NewUploadRepository(pool)

	// Clean up likely conflicts or leftover state (optional, but good for local dev)
	// For a real test suite, we'd use a dedicated test DB or transaction rollback.
	// Here we just insert random IDs so conflicts are unlikely.

	t.Run("list_stale_and_delete", func(t *testing.T) {
		// 1. Create stale PENDING upload (> 24h old)
		staleID := uuid.New()
		stalePending := &domain.Upload{
			ID:           staleID,
			OwnerID:      uuid.New(),
			Purpose:      domain.PurposeEventCover,
			Status:       domain.StatusPending,
			RawObjectKey: "stale-pending",
			CreatedAt:    time.Now().Add(-25 * time.Hour),
			UpdatedAt:    time.Now().Add(-25 * time.Hour),
		}
		require.NoError(t, repo.Create(ctx, stalePending))

		// 2. Create fresh PENDING upload (< 24h old)
		freshID := uuid.New()
		freshPending := &domain.Upload{
			ID:           freshID,
			OwnerID:      uuid.New(),
			Purpose:      domain.PurposeEventCover,
			Status:       domain.StatusPending,
			RawObjectKey: "fresh-pending",
			CreatedAt:    time.Now().Add(-1 * time.Hour),
			UpdatedAt:    time.Now().Add(-1 * time.Hour),
		}
		require.NoError(t, repo.Create(ctx, freshPending))

		// 3. Create stale FAILED upload (> 7 days old)
		failedID := uuid.New()
		failedOld := &domain.Upload{
			ID:           failedID,
			OwnerID:      uuid.New(),
			Purpose:      domain.PurposeEventCover,
			Status:       domain.StatusFailed,
			ErrorMessage: "failed long ago",
			RawObjectKey: "failed-old",
			CreatedAt:    time.Now().Add(-169 * time.Hour), // 7 days + 1 hour
			UpdatedAt:    time.Now().Add(-169 * time.Hour),
		}
		require.NoError(t, repo.Create(ctx, failedOld))

		// 4. Create fresh FAILED upload (< 7 days old)
		failedFreshID := uuid.New()
		failedFresh := &domain.Upload{
			ID:           failedFreshID,
			OwnerID:      uuid.New(),
			Purpose:      domain.PurposeEventCover,
			Status:       domain.StatusFailed,
			ErrorMessage: "failed recently",
			RawObjectKey: "failed-recent",
			CreatedAt:    time.Now().Add(-24 * time.Hour),
			UpdatedAt:    time.Now().Add(-24 * time.Hour),
		}
		require.NoError(t, repo.Create(ctx, failedFresh))

		// 5. List Stale
		// PENDING > 24h, FAILED > 7 days (168h)
		list, err := repo.ListStale(ctx, 24*time.Hour, 168*time.Hour, 100)
		require.NoError(t, err)

		// Should contain stalePending and failedOld
		// Should NOT contain freshPending and failedFresh
		ids := make(map[uuid.UUID]bool)
		for _, u := range list {
			ids[u.ID] = true
		}

		assert.True(t, ids[staleID], "stale pending should be listed")
		assert.True(t, ids[failedID], "old failed should be listed")
		assert.False(t, ids[freshID], "fresh pending should NOT be listed")
		assert.False(t, ids[failedFreshID], "fresh failed should NOT be listed")

		// 6. Delete
		err = repo.Delete(ctx, staleID)
		require.NoError(t, err)

		// Verify deletion
		deleted, err := repo.GetByID(ctx, staleID)
		require.NoError(t, err)
		assert.Nil(t, deleted)

		// Cleanup others
		_ = repo.Delete(ctx, freshID)
		_ = repo.Delete(ctx, failedID)
		_ = repo.Delete(ctx, failedFreshID)
	})
}
