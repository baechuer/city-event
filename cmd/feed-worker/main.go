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

	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("rabbitmq connection failed", slog.String("error", err.Error()))
		return 1
	}
	defer conn.Close()

	consumer := feedprojection.NewConsumer(conn, feedprojection.NewProjector(pool), logger)
	if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("feed worker failed", slog.String("error", err.Error()))
		return 1
	}
	logger.Info("feed worker stopped")
	return 0
}
