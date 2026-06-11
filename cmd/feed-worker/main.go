package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/logging"
	"github.com/baechuer/cityevents/internal/services/feedprojection"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load("feed-worker", os.Getenv)
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

	return runConsumerLoop(ctx, cfg, logger, pool)
}

func runConsumerLoop(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) int {
	backoff := time.Second
	for {
		if err := runConsumerSession(ctx, cfg, logger, pool); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				logger.Info("feed worker stopped")
				return 0
			}
			logger.Error("feed worker session failed; reconnecting", slog.String("error", err.Error()), slog.Duration("backoff", backoff))
		}
		if !sleepContext(ctx, backoff) {
			logger.Info("feed worker stopped")
			return 0
		}
	}
}

func runConsumerSession(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) error {
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	consumer := feedprojection.NewConsumer(conn, feedprojection.NewProjector(pool), logger)
	if err := consumer.Run(ctx); err != nil {
		return err
	}
	return errors.New("feed consumer stopped")
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
