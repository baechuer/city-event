package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"

	"github.com/baechuer/cityevents/services/media-worker/internal/config"
	"github.com/baechuer/cityevents/services/media-worker/internal/sanitizer"
	"github.com/baechuer/cityevents/services/media-worker/internal/storage"
)

// DerivedSizes defines output sizes for each purpose.
var DerivedSizes = map[string][]sanitizer.ResizeConfig{
	"avatar": {
		{Width: 256, Height: 256, Crop: true},
		{Width: 512, Height: 512, Crop: true},
	},
	"event_cover": {
		{Width: 800, Height: 0, Crop: false},
		{Width: 1600, Height: 0, Crop: false},
	},
}

// ProcessImageMessage is the message format from media-service.
type ProcessImageMessage struct {
	UploadID  string `json:"upload_id"`
	ObjectKey string `json:"object_key"`
	Purpose   string `json:"purpose"`
}

// Consumer consumes image processing messages from RabbitMQ.
type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	pool    *pgxpool.Pool
	s3      *storage.S3Client
	cfg     *config.Config
	log     zerolog.Logger
}

// NewConsumer creates a new RabbitMQ consumer.
func NewConsumer(cfg *config.Config, pool *pgxpool.Pool, s3 *storage.S3Client, log zerolog.Logger) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.RabbitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
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
		true, false, false, false, nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare queue
	q, err := ch.QueueDeclare(
		cfg.RabbitQueue,
		true, false, false, false, nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue
	err = ch.QueueBind(q.Name, "media.process.image", cfg.RabbitExchange, false, nil)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	// Set prefetch
	ch.Qos(1, 0, false)

	return &Consumer{
		conn:    conn,
		channel: ch,
		pool:    pool,
		s3:      s3,
		cfg:     cfg,
		log:     log,
	}, nil
}

// Run starts consuming messages.
func (c *Consumer) Run(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		c.cfg.RabbitQueue,
		"",    // consumer tag
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	c.log.Info().Str("queue", c.cfg.RabbitQueue).Msg("media-worker started consuming")

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("channel closed")
			}
			c.processMessage(ctx, msg)
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg amqp.Delivery) {
	var m ProcessImageMessage
	if err := json.Unmarshal(msg.Body, &m); err != nil {
		c.log.Error().Err(err).Msg("failed to unmarshal message")
		msg.Nack(false, false) // Don't requeue malformed messages
		return
	}

	log := c.log.With().Str("upload_id", m.UploadID).Str("purpose", m.Purpose).Logger()
	log.Info().Msg("processing image")

	uploadID, err := uuid.Parse(m.UploadID)
	if err != nil {
		log.Error().Err(err).Msg("invalid upload ID")
		msg.Nack(false, false)
		return
	}

	// Update status to PROCESSING
	if err := c.updateStatus(ctx, uploadID, "PROCESSING", ""); err != nil {
		log.Error().Err(err).Msg("failed to update status")
		msg.Nack(false, true) // Requeue
		return
	}

	// Get raw image from S3
	rawData, err := c.fetchRawImage(ctx, m.ObjectKey)
	if err != nil {
		log.Error().Err(err).Msg("failed to fetch raw image")
		c.updateStatus(ctx, uploadID, "FAILED", err.Error())
		msg.Ack(false)
		return
	}

	// Check size
	if int64(len(rawData)) > c.cfg.MaxUploadSize {
		log.Warn().Int("size", len(rawData)).Msg("image too large")
		c.updateStatus(ctx, uploadID, "FAILED", "file too large")
		c.s3.DeleteRawObject(ctx, m.ObjectKey)
		msg.Ack(false)
		return
	}

	// Get sizes for purpose
	sizes, ok := DerivedSizes[m.Purpose]
	if !ok {
		log.Error().Msg("unknown purpose")
		c.updateStatus(ctx, uploadID, "FAILED", "unknown purpose")
		msg.Ack(false)
		return
	}

	// Process (sanitize, resize)
	results, err := sanitizer.Process(rawData, sizes, c.cfg.MaxImageWidth, c.cfg.MaxImageHeight)
	if err != nil {
		log.Error().Err(err).Msg("failed to process image")
		c.updateStatus(ctx, uploadID, "FAILED", err.Error())
		c.s3.DeleteRawObject(ctx, m.ObjectKey)
		msg.Ack(false)
		return
	}

	// Upload derived images
	derivedKeys := make(map[string]string)
	for size, data := range results {
		key := fmt.Sprintf("derived/%s/%s_%s.jpg", m.Purpose, m.UploadID, size)
		if err := c.s3.PutPublicObject(ctx, key, bytes.NewReader(data), "image/jpeg", int64(len(data))); err != nil {
			log.Error().Err(err).Str("size", size).Msg("failed to upload derived image")
			c.updateStatus(ctx, uploadID, "FAILED", err.Error())
			msg.Nack(false, true) // Requeue
			return
		}
		derivedKeys[size] = key
		log.Info().Str("size", size).Str("key", key).Msg("uploaded derived image")
	}

	// Update database with derived keys
	if err := c.updateDerivedKeys(ctx, uploadID, derivedKeys); err != nil {
		log.Error().Err(err).Msg("failed to update derived keys")
		msg.Nack(false, true) // Requeue
		return
	}

	// Delete raw image (optional, for cleanup)
	// c.s3.DeleteRawObject(ctx, m.ObjectKey)

	log.Info().Msg("image processing complete")
	msg.Ack(false)
}

func (c *Consumer) fetchRawImage(ctx context.Context, objectKey string) ([]byte, error) {
	reader, err := c.s3.GetObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (c *Consumer) updateStatus(ctx context.Context, id uuid.UUID, status, errMsg string) error {
	if errMsg != "" {
		_, err := c.pool.Exec(ctx, `
			UPDATE media_uploads SET status = $2, error_message = $3, updated_at = $4 WHERE id = $1
		`, id, status, errMsg, time.Now())
		return err
	}
	_, err := c.pool.Exec(ctx, `
		UPDATE media_uploads SET status = $2, updated_at = $3 WHERE id = $1
	`, id, status, time.Now())
	return err
}

func (c *Consumer) updateDerivedKeys(ctx context.Context, id uuid.UUID, derivedKeys map[string]string) error {
	keysJSON, _ := json.Marshal(derivedKeys)
	_, err := c.pool.Exec(ctx, `
		UPDATE media_uploads SET derived_keys = $2, status = 'READY', updated_at = $3 WHERE id = $1
	`, id, keysJSON, time.Now())
	return err
}

// Close closes the consumer.
func (c *Consumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}
