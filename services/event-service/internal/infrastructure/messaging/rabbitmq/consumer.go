package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

// JoinEventMessage represents the message from join-service
type JoinEventMessage struct {
	EventID   string    `json:"event_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	TraceID   string    `json:"trace_id"`
}

// Consumer listens to join.* events and updates event participation counts
type Consumer struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	queue    string
	service  *event.Service
	exchange string
}

// NewConsumer creates a new RabbitMQ consumer for join events
func NewConsumer(rabbitURL, exchange string, service *event.Service) (*Consumer, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// 1. Declare Main Exchange (Topic)
	err = ch.ExchangeDeclare(
		exchange, "topic", true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// 2. Declare DLX (Fanout)
	dlxName := "events.dlx"
	err = ch.ExchangeDeclare(
		dlxName, "fanout", true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare dlx: %w", err)
	}

	// 3. Declare DLQ and bind to DLX
	dlqName := "event-service.join-events.dlq"
	_, err = ch.QueueDeclare(
		dlqName, true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare dlq: %w", err)
	}
	err = ch.QueueBind(dlqName, "", dlxName, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to bind dlq: %w", err)
	}

	// 4. Declare Main Queue with DLX configuration
	queueName := "event-service.join-events"
	mainQArgs := amqp.Table{
		"x-dead-letter-exchange": dlxName, // If rejected (Nack), go to DLX -> DLQ
	}
	q, err := ch.QueueDeclare(
		queueName, true, false, false, false, mainQArgs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare main queue: %w", err)
	}

	// 5. Declare Retry Queue with TTL and Main Queue as destination
	retryQueueName := "event-service.join-events.retry"
	retryQArgs := amqp.Table{
		"x-dead-letter-exchange":    "",        // Default exchange
		"x-dead-letter-routing-key": queueName, // Route back to Main Queue
		"x-message-ttl":             5000,      // 5 seconds
	}
	_, err = ch.QueueDeclare(
		retryQueueName, true, false, false, false, retryQArgs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare retry queue: %w", err)
	}

	// Bind Main Queue to Main Exchange
	routingKeys := []string{"join.created", "join.canceled"}
	for _, key := range routingKeys {
		err = ch.QueueBind(q.Name, key, exchange, false, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to bind queue to %s: %w", key, err)
		}
	}

	return &Consumer{
		conn:     conn,
		channel:  ch,
		queue:    q.Name,
		service:  service,
		exchange: exchange,
	}, nil
}

// Start begins consuming messages
func (c *Consumer) Start(ctx context.Context) {
	go c.consume(ctx)
	log.Info().
		Str("queue", c.queue).
		Str("exchange", c.exchange).
		Msg("join events consumer started")
}

func (c *Consumer) consume(ctx context.Context) {
	msgs, err := c.channel.Consume(
		c.queue, "", false, false, false, false, nil,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to start consuming")
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("consumer shutting down")
			return
		case msg, ok := <-msgs:
			if !ok {
				log.Warn().Msg("consumer channel closed")
				return
			}
			c.handleMessage(msg)
		}
	}
}

func (c *Consumer) handleMessage(msg amqp.Delivery) {
	// Determine effective routing key (original or current)
	routingKey := msg.RoutingKey
	if val, ok := msg.Headers["x-original-routing-key"].(string); ok {
		routingKey = val
	}

	log.Debug().
		Str("routing_key", routingKey).
		Str("message_id", msg.MessageId).
		Msg("received join event")

	var joinMsg JoinEventMessage
	if err := json.Unmarshal(msg.Body, &joinMsg); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal join event")
		msg.Nack(false, false) // Poison message -> DLQ
		return
	}

	eventID, err := uuid.Parse(joinMsg.EventID)
	if err != nil {
		log.Error().Err(err).Str("event_id", joinMsg.EventID).Msg("invalid event_id")
		msg.Nack(false, false) // Poison message -> DLQ
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Process based on routing key
	switch routingKey {
	case "join.created":
		err = c.service.IncrementParticipantCount(ctx, eventID)
	case "join.canceled":
		err = c.service.DecrementParticipantCount(ctx, eventID)
	default:
		log.Warn().Str("routing_key", routingKey).Msg("unknown routing key")
		msg.Ack(false)
		return
	}

	if err != nil {
		// 1. Check if it's a transient "Not Found" vs permanent
		// Currently treating Not Found as something to just drop to avoid loops,
		// BUT if we want to retry (maybe event creation is lagging?), we can remove this block.
		// However, based on user request to "clear message", they likely want to Drop or DLQ.
		// Given we have DLQ now, maybe we should Nack to DLQ instead of Ack?
		// Let's stick to Ack for NotFound to be clean, as per previous fix,
		// OR let it follow retry logic if we think it might eventually appear.
		// Let's keep the DROP logic for explicit Not Found to avoid waste.
		var appErr *domain.AppError
		if errors.As(err, &appErr) && appErr.Code == domain.CodeNotFound {
			log.Warn().
				Str("event_id", joinMsg.EventID).
				Msg("event not found, dropping message")
			msg.Ack(false)
			return
		}

		// 2. Retry Logic
		retryCount := 0
		if val, ok := msg.Headers["x-retry-count"].(int32); ok {
			retryCount = int(val)
		}

		if retryCount < 3 {
			log.Warn().
				Err(err).
				Int("retry_count", retryCount).
				Msg("processing failed, scheduling retry")

			// Publish to Retry Queue
			headers := make(amqp.Table)
			for k, v := range msg.Headers {
				headers[k] = v
			}
			headers["x-retry-count"] = int32(retryCount + 1)
			headers["x-original-routing-key"] = routingKey

			pubErr := c.channel.Publish(
				"",                                // Default Exchange
				"event-service.join-events.retry", // Routing Key = Retry Queue Name
				false,
				false,
				amqp.Publishing{
					ContentType: msg.ContentType,
					Body:        msg.Body,
					Headers:     headers,
					MessageId:   msg.MessageId,
				},
			)

			if pubErr != nil {
				log.Error().Err(pubErr).Msg("failed to publish to retry queue")
				msg.Nack(false, false) // Failed to retry -> DLQ
			} else {
				msg.Ack(false) // Handled via retry
			}
			return
		}

		// 3. Max Retries Reached -> DLQ
		log.Error().
			Err(err).
			Str("event_id", joinMsg.EventID).
			Msg("max retries reached, sending to DLQ")
		msg.Nack(false, false) // Requeue=false + DLX configured = DLQ
		return
	}

	log.Info().
		Str("event_id", joinMsg.EventID).
		Str("routing_key", routingKey).
		Msg("participant count updated")
	msg.Ack(false)
}

// Close closes the consumer connection
func (c *Consumer) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
