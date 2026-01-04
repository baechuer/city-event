package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/baechuer/cityevents/services/media-worker/internal/config"
	"github.com/baechuer/cityevents/services/media-worker/internal/consumer"
	"github.com/baechuer/cityevents/services/media-worker/internal/storage"
)

func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg := config.Load()
	log.Info().Msg("starting media-worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	// Initialize S3 client
	s3Client, err := storage.NewS3Client(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create S3 client")
	}

	// Initialize consumer
	cons, err := consumer.NewConsumer(cfg, pool, s3Client, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create consumer")
	}
	defer cons.Close()

	// Start consuming in goroutine
	go func() {
		if err := cons.Run(ctx); err != nil {
			log.Error().Err(err).Msg("consumer error")
		}
	}()

	log.Info().Msg("media-worker started")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down media-worker")
	cancel()
}
