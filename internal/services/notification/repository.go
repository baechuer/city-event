package notification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool         *pgxpool.Pool
	consumerName string
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, consumerName: DefaultConsumerName}
}

type ProcessResult struct {
	Notification Notification
	Duplicate    bool
	Ignored      bool
}

func (r *Repository) ProcessEnvelope(ctx context.Context, envelope messaging.Envelope, provider Provider) (ProcessResult, error) {
	if err := envelope.Validate(); err != nil {
		return ProcessResult{}, err
	}
	decision, err := Decide(envelope)
	if errors.Is(err, ErrUnsupportedRoutingKey) {
		observability.RecordConsumerMessage(r.consumerName, envelope.RoutingKey, "ignored")
		return ProcessResult{Ignored: true}, nil
	}
	if err != nil {
		observability.RecordConsumerMessage(r.consumerName, envelope.RoutingKey, "failed")
		return ProcessResult{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProcessResult{}, err
	}
	defer rollback(ctx, tx)

	claimed, err := claimMessage(ctx, tx, r.consumerName, envelope)
	if err != nil {
		return ProcessResult{}, err
	}
	if !claimed {
		notification, err := r.findByMessageIDTx(ctx, tx, envelope.MessageID)
		if err != nil {
			return ProcessResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return ProcessResult{}, err
		}
		observability.RecordConsumerMessage(r.consumerName, envelope.RoutingKey, "duplicate")
		return ProcessResult{Notification: notification, Duplicate: true}, nil
	}

	now := time.Now().UTC()
	notification := Notification{
		ID:              NewID(),
		MessageID:       envelope.MessageID,
		RecipientUserID: decision.RecipientUserID,
		RecipientEmail:  decision.RecipientEmail,
		RoutingKey:      envelope.RoutingKey,
		Subject:         decision.Subject,
		Body:            decision.Body,
		Status:          StatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := insertNotification(ctx, tx, notification); err != nil {
		return ProcessResult{}, err
	}

	result, sendErr := provider.Send(ctx, Message{
		ID:      notification.ID,
		To:      notification.RecipientEmail,
		Subject: notification.Subject,
		Body:    notification.Body,
	})
	if sendErr != nil {
		notification.Status = StatusFailed
		notification.LastError = trimError(sendErr)
	} else {
		notification.Status = StatusSent
	}
	notification.UpdatedAt = time.Now().UTC()

	if err := updateStatus(ctx, tx, notification); err != nil {
		return ProcessResult{}, err
	}
	delivery := Delivery{
		ID:                NewID(),
		NotificationID:    notification.ID,
		Provider:          "smtp",
		Status:            notification.Status,
		ProviderMessageID: result.ProviderMessageID,
		Error:             notification.LastError,
		CreatedAt:         notification.UpdatedAt,
	}
	if err := insertDelivery(ctx, tx, delivery); err != nil {
		return ProcessResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProcessResult{}, err
	}
	observability.RecordConsumerMessage(r.consumerName, envelope.RoutingKey, "processed")
	return ProcessResult{Notification: notification}, nil
}

func (r *Repository) NotificationCount(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&count)
	return count, err
}

func (r *Repository) DeliveryCount(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM notification_deliveries`).Scan(&count)
	return count, err
}

func (r *Repository) ProcessedCount(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM notification_processed_messages WHERE consumer_name = $1`, r.consumerName).Scan(&count)
	return count, err
}

func (r *Repository) GetByMessageID(ctx context.Context, messageID string) (Notification, error) {
	return scanNotification(r.pool.QueryRow(ctx, `
		SELECT id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, created_at, updated_at
		FROM notifications
		WHERE message_id = $1
	`, strings.TrimSpace(messageID)))
}

func claimMessage(ctx context.Context, tx pgx.Tx, consumerName string, envelope messaging.Envelope) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO notification_processed_messages (consumer_name, message_id, routing_key, processed_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (consumer_name, message_id) DO NOTHING
	`, consumerName, envelope.MessageID, envelope.RoutingKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func insertNotification(ctx context.Context, tx pgx.Tx, notification Notification) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO notifications (id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, notification.ID, notification.MessageID, notification.RecipientUserID, notification.RecipientEmail, notification.RoutingKey, notification.Subject, notification.Body, notification.Status, notification.LastError, notification.CreatedAt, notification.UpdatedAt)
	return err
}

func updateStatus(ctx context.Context, tx pgx.Tx, notification Notification) error {
	_, err := tx.Exec(ctx, `
		UPDATE notifications
		SET status = $2, last_error = $3, updated_at = $4
		WHERE id = $1
	`, notification.ID, notification.Status, notification.LastError, notification.UpdatedAt)
	return err
}

func insertDelivery(ctx context.Context, tx pgx.Tx, delivery Delivery) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO notification_deliveries (id, notification_id, provider, status, provider_message_id, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, delivery.ID, delivery.NotificationID, delivery.Provider, delivery.Status, delivery.ProviderMessageID, delivery.Error, delivery.CreatedAt)
	return err
}

func (r *Repository) findByMessageIDTx(ctx context.Context, tx pgx.Tx, messageID string) (Notification, error) {
	return scanNotification(tx.QueryRow(ctx, `
		SELECT id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, created_at, updated_at
		FROM notifications
		WHERE message_id = $1
	`, messageID))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNotification(row scanner) (Notification, error) {
	var notification Notification
	err := row.Scan(&notification.ID, &notification.MessageID, &notification.RecipientUserID, &notification.RecipientEmail, &notification.RoutingKey, &notification.Subject, &notification.Body, &notification.Status, &notification.LastError, &notification.CreatedAt, &notification.UpdatedAt)
	return notification, err
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
