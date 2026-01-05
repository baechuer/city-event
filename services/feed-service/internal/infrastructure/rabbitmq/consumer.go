package rabbitmq

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/infrastructure/postgres"
	amqp "github.com/rabbitmq/amqp091-go"
)

type EventPublishedPayload struct {
	EventID       string    `json:"event_id"`
	OwnerID       string    `json:"owner_id"`
	Title         string    `json:"title"`
	City          string    `json:"city"` // e.g. "Sydney"
	Category      string    `json:"category"`
	StartTime     time.Time `json:"start_time"`
	Status        string    `json:"status"`
	CoverImageIDs []string  `json:"cover_image_ids"`
}

type DomainEventEnvelope struct {
	MessageID  string          `json:"message_id"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type Consumer struct {
	connURL   string
	repo      *postgres.TrackRepo // Use TrackRepo or new Repo to store/index events
	reconnect int
}

func NewConsumer(connURL string, repo *postgres.TrackRepo) *Consumer {
	return &Consumer{
		connURL: connURL,
		repo:    repo,
	}
}

func (c *Consumer) Start(ctx context.Context) {
	log.Println("Consumer.Start called")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := c.connectAndConsume(ctx); err != nil {
				log.Printf("rabbit consumer error: %v, retrying in 5s...", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (c *Consumer) connectAndConsume(ctx context.Context) error {
	conn, err := amqp.Dial(c.connURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// Declare exchange (must match event-service)
	err = ch.ExchangeDeclare(
		"cityevents", // name - must match event-service's exchange
		"topic",      // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return err
	}

	q, err := ch.QueueDeclare(
		"feed-service.events", // name
		true,                  // durable
		false,                 // delete when unused
		false,                 // exclusive
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		return err
	}

	// Bind to event.published
	err = ch.QueueBind(
		q.Name,
		"event.published",
		"cityevents",
		false,
		nil,
	)
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return err
	}

	log.Println("feed-service consumer started")

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-msgs:
			if !ok {
				return amqp.ErrClosed
			}

			if err := c.handleMessage(ctx, d.Body); err != nil {
				log.Printf("failed to handle message: %v", err)
				// Negative Ack with requeue=false (dead letter)
				_ = d.Nack(false, false)
			} else {
				_ = d.Ack(false)
			}
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
	// event-service sends "Payload" as object, not raw bytes in some versions,
	// but the struct above defined Payload as RawMessage.
	// Let's verify event-service payload structure.

	// Quick fix: EventService outbox uses:
	/*
	   type DomainEventEnvelope[T any] struct {
	       ...
	       Payload T `json:"payload"`
	   }
	*/
	// So unmarshalling explicit struct into another struct is tricky if using generics.
	// Better to just unmarshal full envelope matching expected structure.

	// Let's iterate using simple map to avoid type issues or define specific envelope.
	type RealEnvelope struct {
		Payload EventPublishedPayload `json:"payload"`
	}
	var env RealEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}

	log.Printf("received event published: %s (%s)", env.Payload.EventID, env.Payload.City)

	// TODO: We need a Postgres method to UPSERT this into event_index
	return c.repo.IndexEvent(ctx, env.Payload.EventID, env.Payload.OwnerID, env.Payload.Title, env.Payload.City, env.Payload.Category, env.Payload.StartTime, env.Payload.Status, env.Payload.CoverImageIDs)
}
