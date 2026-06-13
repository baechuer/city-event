package feedprojection

import (
	"context"
	"log/slog"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/baechuer/cityevents/internal/platform/observability"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn      *amqp.Connection
	projector *Projector
	logger    *slog.Logger
}

func NewConsumer(conn *amqp.Connection, projector *Projector, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Consumer{conn: conn, projector: projector, logger: logger}
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
	if err := ch.Confirm(false); err != nil {
		return err
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(messaging.FeedQueue, "", false, false, false, false, nil)
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
				c.logger.Error("feed projection message failed", slog.String("message_id", delivery.MessageId), slog.String("error", err.Error()))
				action, retryErr := messaging.PublishFailedDelivery(ctx, ch, confirms, messaging.FeedQueue, delivery, err)
				if retryErr != nil {
					c.logger.Error("feed projection retry publish failed", slog.String("message_id", delivery.MessageId), slog.String("error", retryErr.Error()))
					_ = delivery.Nack(false, true)
					continue
				}
				observability.RecordConsumerMessage(DefaultConsumerName, messaging.DeliveryRoutingKey(delivery), string(action))
				_ = delivery.Ack(false)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	ctx = messaging.ContextWithAMQPTraceHeaders(ctx, delivery.Headers)
	envelope, err := messaging.DecodeEnvelope(delivery.Body)
	if err != nil {
		return err
	}
	return c.projector.HandleEnvelope(ctx, envelope)
}
