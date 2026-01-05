package cleanup

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/baechuer/cityevents/services/media-service/internal/repository"
	"github.com/baechuer/cityevents/services/media-service/internal/storage"
)

// Cleaner cleans up stale uploads.
type Cleaner struct {
	repo *repository.UploadRepository
	s3   *storage.S3Client
	log  zerolog.Logger
}

// NewCleaner creates a new Cleaner.
func NewCleaner(repo *repository.UploadRepository, s3 *storage.S3Client, log zerolog.Logger) *Cleaner {
	return &Cleaner{
		repo: repo,
		s3:   s3,
		log:  log,
	}
}

// Run starts the cleanup loop.
func (c *Cleaner) Run(ctx context.Context) {
	c.log.Info().Msg("cleanup worker started")
	ticker := time.NewTicker(1 * time.Hour) // Run every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Info().Msg("cleanup worker stopped")
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *Cleaner) cleanup(ctx context.Context) {
	c.log.Info().Msg("starting cleanup of stale uploads")

	// PENDING > 24 hours, FAILED > 7 days
	uploads, err := c.repo.ListStale(ctx, 24*time.Hour, 7*24*time.Hour, 100)
	if err != nil {
		c.log.Error().Err(err).Msg("failed to list stale uploads")
		return
	}

	if len(uploads) == 0 {
		c.log.Info().Msg("no stale uploads found")
		return
	}

	c.log.Info().Int("count", len(uploads)).Msg("found stale uploads")

	for _, u := range uploads {
		// 1. Delete from S3 (Raw Object)
		if u.RawObjectKey != "" {
			if err := c.s3.DeleteRawObject(ctx, u.RawObjectKey); err != nil {
				c.log.Error().Err(err).Str("id", u.ID.String()).Str("key", u.RawObjectKey).Msg("failed to delete raw object")
				// Continue to delete from DB anyway? Or retry?
				// If we fail to delete from S3, the file remains but DB record is gone.
				// This leaves trash. Better to keep DB record if S3 delete fails?
				// But presigned URLs expire, so it's just garbage.
			}
		}

		// 2. Delete from DB
		if err := c.repo.Delete(ctx, u.ID); err != nil {
			c.log.Error().Err(err).Str("id", u.ID.String()).Msg("failed to delete upload record")
		} else {
			c.log.Info().Str("id", u.ID.String()).Msg("deleted stale upload")
		}
	}
}
