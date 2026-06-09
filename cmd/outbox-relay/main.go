package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/logging"
	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/baechuer/cityevents/internal/services/outboxrelay"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load("outbox-relay", os.Getenv)
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

	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("rabbitmq connection failed", slog.String("error", err.Error()))
		return 1
	}
	defer conn.Close()

	publisher, err := messaging.NewConfirmingPublisher(conn)
	if err != nil {
		logger.Error("rabbitmq publisher setup failed", slog.String("error", err.Error()))
		return 1
	}
	defer publisher.Close()

	relay := outboxrelay.NewRelay(outboxrelay.NewStore(pool), publisher)
	batchSize := envInt("OUTBOX_RELAY_BATCH_SIZE", outboxrelay.DefaultBatchSize)
	if truthy(os.Getenv("OUTBOX_RELAY_RUN_ONCE")) {
		published, err := relay.PublishBatch(ctx, batchSize)
		if err != nil {
			logger.Error("outbox publish batch failed", slog.Int("published", published), slog.String("error", err.Error()))
			return 1
		}
		logger.Info("outbox publish batch completed", slog.Int("published", published))
		return 0
	}

	interval := envDuration("OUTBOX_RELAY_POLL_INTERVAL", time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		published, err := relay.PublishBatch(ctx, batchSize)
		if err != nil {
			logger.Error("outbox publish batch failed", slog.Int("published", published), slog.String("error", err.Error()))
		} else if published > 0 {
			logger.Info("outbox publish batch completed", slog.Int("published", published))
		}

		select {
		case <-ctx.Done():
			logger.Info("outbox relay stopped")
			return 0
		case <-ticker.C:
		}
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
