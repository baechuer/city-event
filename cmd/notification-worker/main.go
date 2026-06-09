package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("rabbitmq connection failed", slog.String("error", err.Error()))
		return 1
	}
	defer conn.Close()

	consumer := notification.NewConsumer(conn, notification.NewRepository(pool), notification.NewSMTPProvider(cfg.SMTPAddr), logger)
	if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("notification worker failed", slog.String("error", err.Error()))
		return 1
	}
	logger.Info("notification worker stopped")
	return 0
}
