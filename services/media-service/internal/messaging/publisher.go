package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"

	"github.com/baechuer/cityevents/services/media-service/internal/config"
)

// Publisher publishes messages to RabbitMQ.
type Publisher struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	exchange string
	log      zerolog.Logger
}

// ProcessImageMessage is the message sent to the worker.
type ProcessImageMessage struct {
	UploadID  string `json:"upload_id"`
	ObjectKey string `json:"object_key"`
	Purpose   string `json:"purpose"`
}

// NewPublisher creates a new RabbitMQ publisher.
func NewPublisher(cfg *config.Config, log zerolog.Logger) (*Publisher, error) {
	var conn *amqp.Connection
	var err error

	// Retry connection for up to 30 seconds
	for i := 0; i < 6; i++ {
		conn, err = amqp.Dial(cfg.RabbitURL)
		if err == nil {
			break
		}
		log.Warn().Err(err).Msgf("failed to connect to RabbitMQ, retrying in 5s... (%d/6)", i+1)
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ after retries: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchange
	err = ch.ExchangeDeclare(
		cfg.RabbitExchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	return &Publisher{
		conn:     conn,
		channel:  ch,
		exchange: cfg.RabbitExchange,
		log:      log,
	}, nil
}

// PublishProcessImage publishes a message to process an uploaded image.
func (p *Publisher) PublishProcessImage(ctx context.Context, uploadID, objectKey, purpose string) error {
	msg := ProcessImageMessage{
		UploadID:  uploadID,
		ObjectKey: objectKey,
		Purpose:   purpose,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = p.channel.PublishWithContext(ctx,
		p.exchange,
		"media.process.image", // routing key
		false,                 // mandatory
		false,                 // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	p.log.Info().Str("upload_id", uploadID).Msg("published process image message")
	return nil
}

// Close closes the publisher connection.
func (p *Publisher) Close() {
	if p.channel != nil {
		p.channel.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
}
