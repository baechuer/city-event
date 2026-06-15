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
	"github.com/baechuer/cityevents/internal/services/notification"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load("notification-worker", os.Getenv)
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

	repo := notification.NewRepository(pool)
	go runDeliveryLoop(ctx, logger, notification.NewDeliveryWorker(repo, notification.NewSMTPProvider(cfg.SMTPAddr)))

	return runConsumerLoop(ctx, cfg, logger, repo)
}

func runConsumerLoop(ctx context.Context, cfg config.Config, logger *slog.Logger, repo *notification.Repository) int {
	backoff := time.Second
	for {
		if err := runConsumerSession(ctx, cfg, logger, repo); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				logger.Info("notification worker stopped")
				return 0
			}
			logger.Error("notification worker session failed; reconnecting", slog.String("error", err.Error()), slog.Duration("backoff", backoff))
		}
		if !sleepContext(ctx, backoff) {
			logger.Info("notification worker stopped")
			return 0
		}
	}
}

func runConsumerSession(ctx context.Context, cfg config.Config, logger *slog.Logger, repo *notification.Repository) error {
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	consumer := notification.NewConsumer(conn, repo, logger)
	if err := consumer.Run(ctx); err != nil {
		return err
	}
	return errors.New("notification consumer stopped")
}

func runDeliveryLoop(ctx context.Context, logger *slog.Logger, worker *notification.DeliveryWorker) {
	for {
		processed, err := worker.ProcessOne(ctx)
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			logger.Info("notification delivery stopped")
			return
		}
		if err != nil {
			logger.Error("notification delivery failed", slog.String("error", err.Error()))
		}
		if processed {
			continue
		}
		if !sleepContext(ctx, time.Second) {
			logger.Info("notification delivery stopped")
			return
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
