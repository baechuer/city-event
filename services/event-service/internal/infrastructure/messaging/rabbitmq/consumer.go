package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
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

	// Declare exchange (should already exist, but idempotent)
	err = ch.ExchangeDeclare(
		exchange, // name
		"topic",  // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare queue for this consumer
	queueName := "event-service.join-events"
	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to exchange with routing keys for join events
	routingKeys := []string{"join.created", "join.canceled"}
	for _, key := range routingKeys {
		err = ch.QueueBind(
			q.Name,   // queue name
			key,      // routing key
			exchange, // exchange
			false,
			nil,
		)
		if err != nil {
			ch.Close()
			conn.Close()
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
		c.queue, // queue
		"",      // consumer tag
		false,   // auto-ack (we'll ack manually)
		false,   // exclusive
		false,   // no-local
		false,   // no-wait
		nil,     // args
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
	log.Debug().
		Str("routing_key", msg.RoutingKey).
		Str("message_id", msg.MessageId).
		Msg("received join event")

	var joinMsg JoinEventMessage
	if err := json.Unmarshal(msg.Body, &joinMsg); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal join event")
		msg.Nack(false, false) // don't requeue malformed messages
		return
	}

	eventID, err := uuid.Parse(joinMsg.EventID)
	if err != nil {
		log.Error().Err(err).Str("event_id", joinMsg.EventID).Msg("invalid event_id")
		msg.Nack(false, false)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Process based on routing key
	switch msg.RoutingKey {
	case "join.created":
		err = c.service.IncrementParticipantCount(ctx, eventID)
	case "join.canceled":
		err = c.service.DecrementParticipantCount(ctx, eventID)
	default:
		log.Warn().Str("routing_key", msg.RoutingKey).Msg("unknown routing key")
		msg.Ack(false)
		return
	}

	if err != nil {
		log.Error().
			Err(err).
			Str("event_id", joinMsg.EventID).
			Str("routing_key", msg.RoutingKey).
			Msg("failed to update participant count")
		msg.Nack(false, true) // requeue for retry
		return
	}

	log.Info().
		Str("event_id", joinMsg.EventID).
		Str("routing_key", msg.RoutingKey).
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
