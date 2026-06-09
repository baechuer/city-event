package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/logging"
	"github.com/baechuer/cityevents/internal/services/media"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load("media-worker", os.Getenv)
	if err != nil {
		slog.Error("config load failed", slog.String("error", err.Error()))
		return 1
	}
	logger := logging.New(cfg.Service.Name, cfg.Environment)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("postgres connection failed", slog.String("error", err.Error()))
		return 1
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("postgres ping failed", slog.String("error", err.Error()))
		return 1
	}
	storage, err := media.NewMinIOStorage(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey)
	if err != nil {
		logger.Error("minio setup failed", slog.String("error", err.Error()))
		return 1
	}
	worker := media.NewWorker(media.NewPostgresRepository(pool), storage)

	if truthy(os.Getenv("MEDIA_WORKER_RUN_ONCE")) {
		processed, err := worker.ProcessOne(ctx)
		if err != nil {
			logger.Error("media process failed", slog.String("error", err.Error()))
			return 1
		}
		logger.Info("media process completed", slog.Bool("processed", processed))
		return 0
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		processed, err := worker.ProcessOne(ctx)
		if err != nil {
			logger.Error("media process failed", slog.String("error", err.Error()))
		} else if processed {
			logger.Info("media process completed")
		}
		select {
		case <-ctx.Done():
			logger.Info("media worker stopped")
			return 0
		case <-ticker.C:
		}
	}
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
