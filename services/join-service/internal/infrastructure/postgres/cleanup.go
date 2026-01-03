// PATH: services/join-service/internal/infrastructure/postgres/cleanup.go
package postgres

import (
	"context"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/pkg/logger"
)

// StartIdempotencyKeyCleanup starts a background goroutine that periodically
// deletes expired idempotency keys to prevent unbounded table growth.
// The cleanup runs every hour and deletes keys where expires_at < NOW().
func (r *Repository) StartIdempotencyKeyCleanup(ctx context.Context) {
	go func() {
		log := logger.Logger.With().Str("component", "idempotency_cleanup").Logger()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run once immediately on startup
		r.cleanupExpiredKeys(ctx)

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("stopped")
				return
			case <-ticker.C:
				r.cleanupExpiredKeys(ctx)
			}
		}
	}()
}

func (r *Repository) cleanupExpiredKeys(ctx context.Context) {
	result, err := r.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE expires_at < NOW()`)
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("idempotency key cleanup failed")
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected > 0 {
		logger.Logger.Info().Int64("deleted", rowsAffected).Msg("idempotency keys cleaned up")
	}
}
