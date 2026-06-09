package notification

import (
	"context"
	"log/slog"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn     *amqp.Connection
	repo     *Repository
	provider Provider
	logger   *slog.Logger
}

func NewConsumer(conn *amqp.Connection, repo *Repository, provider Provider, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Consumer{conn: conn, repo: repo, provider: provider, logger: logger}
}

func (c *Consumer) Run(ctx context.Context) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := messaging.DeclareTopology(ch); err != nil {
		return err
	}
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(messaging.NotificationQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}
			if err := c.handleDelivery(ctx, delivery); err != nil {
				c.logger.Error("notification message failed", slog.String("message_id", delivery.MessageId), slog.String("error", err.Error()))
				_ = delivery.Nack(false, true)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	envelope, err := messaging.DecodeEnvelope(delivery.Body)
	if err != nil {
		return err
	}
	_, err = c.repo.ProcessEnvelope(ctx, envelope, c.provider)
	return err
}
