package main

import (
	"context"
	"errors"
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

	batchSize := envInt("OUTBOX_RELAY_BATCH_SIZE", outboxrelay.DefaultBatchSize)
	if truthy(os.Getenv("OUTBOX_RELAY_RUN_ONCE")) {
		publisher, cleanup, err := newPublisher(cfg.RabbitMQURL)
		if err != nil {
			logger.Error("rabbitmq publisher setup failed", slog.String("error", err.Error()))
			return 1
		}
		defer cleanup()
		relay := outboxrelay.NewRelay(outboxrelay.NewStore(pool), publisher)
		published, err := relay.PublishBatch(ctx, batchSize)
		if err != nil {
			logger.Error("outbox publish batch failed", slog.Int("published", published), slog.String("error", err.Error()))
			return 1
		}
		logger.Info("outbox publish batch completed", slog.Int("published", published))
		return 0
	}

	interval := envDuration("OUTBOX_RELAY_POLL_INTERVAL", time.Second)
	reconnectBackoff := envDuration("RABBITMQ_RECONNECT_BACKOFF", time.Second)
	return runRelayLoop(ctx, cfg, logger, pool, batchSize, interval, reconnectBackoff)
}

func newPublisher(rabbitURL string) (*messaging.ConfirmingPublisher, func(), error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, nil, err
	}
	publisher, err := messaging.NewConfirmingPublisher(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	cleanup := func() {
		_ = publisher.Close()
		_ = conn.Close()
	}
	return publisher, cleanup, nil
}

func runRelayLoop(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, batchSize int, interval time.Duration, reconnectBackoff time.Duration) int {
	if reconnectBackoff <= 0 {
		reconnectBackoff = time.Second
	}
	for {
		if err := runRelaySession(ctx, cfg, logger, pool, batchSize, interval); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				logger.Info("outbox relay stopped")
				return 0
			}
			logger.Error("outbox relay session failed; reconnecting", slog.String("error", err.Error()), slog.Duration("backoff", reconnectBackoff))
		}
		if !sleepContext(ctx, reconnectBackoff) {
			logger.Info("outbox relay stopped")
			return 0
		}
	}
}

func runRelaySession(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, batchSize int, interval time.Duration) error {
	publisher, cleanup, err := newPublisher(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer cleanup()

	relay := outboxrelay.NewRelay(outboxrelay.NewStore(pool), publisher)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		published, err := relay.PublishBatch(ctx, batchSize)
		if err != nil {
			logger.Error("outbox publish batch failed", slog.Int("published", published), slog.String("error", err.Error()))
			return err
		} else if published > 0 {
			logger.Info("outbox publish batch completed", slog.Int("published", published))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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
