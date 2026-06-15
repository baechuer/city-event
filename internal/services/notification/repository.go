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

const DefaultDeliveryLease = 30 * time.Second
const MaxDeliveryAttempts = 5

var ErrStaleDeliveryClaim = errors.New("notification delivery claim is stale")

var deliveryRetryBackoffs = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

type ProcessResult struct {
	Notification Notification
	Duplicate    bool
	Ignored      bool
}

func (r *Repository) ProcessEnvelope(ctx context.Context, envelope messaging.Envelope) (ProcessResult, error) {
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
		IdempotencyKey:  envelope.MessageID,
		NextAttemptAt:   now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := insertNotification(ctx, tx, notification); err != nil {
		return ProcessResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProcessResult{}, err
	}
	observability.RecordConsumerMessage(r.consumerName, envelope.RoutingKey, "processed")
	return ProcessResult{Notification: notification}, nil
}

type DeliveryWorker struct {
	repo     *Repository
	provider Provider
	now      func() time.Time
	lease    time.Duration
}

func NewDeliveryWorker(repo *Repository, provider Provider) *DeliveryWorker {
	return &DeliveryWorker{
		repo:     repo,
		provider: provider,
		now:      time.Now,
		lease:    DefaultDeliveryLease,
	}
}

func (w *DeliveryWorker) ProcessOne(ctx context.Context) (bool, error) {
	now := w.now().UTC()
	notification, ok, err := w.repo.ClaimNextDelivery(ctx, now, w.lease)
	if err != nil || !ok {
		return ok, err
	}
	result, sendErr := w.provider.Send(ctx, Message{
		ID:             notification.ID,
		IdempotencyKey: notification.IdempotencyKey,
		To:             notification.RecipientEmail,
		Subject:        notification.Subject,
		Body:           notification.Body,
	})
	if sendErr != nil {
		if _, err := w.repo.MarkDeliveryFailed(ctx, notification, sendErr, w.now().UTC()); err != nil {
			return true, errors.Join(sendErr, err)
		}
		return true, sendErr
	}
	if _, err := w.repo.MarkDeliverySent(ctx, notification, result, w.now().UTC()); err != nil {
		return true, err
	}
	return true, nil
}

func (r *Repository) ClaimNextDelivery(ctx context.Context, now time.Time, lease time.Duration) (Notification, bool, error) {
	if lease <= 0 {
		lease = DefaultDeliveryLease
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Notification{}, false, err
	}
	defer rollback(ctx, tx)

	notification, err := scanNotification(tx.QueryRow(ctx, `
		SELECT id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, idempotency_key, delivery_attempts, next_attempt_at, locked_until, created_at, updated_at
		FROM notifications
		WHERE delivery_attempts < $2
		  AND (
		    (status IN ('PENDING', 'FAILED') AND next_attempt_at <= $1)
		    OR (status = 'PROCESSING' AND locked_until <= $1)
		  )
		ORDER BY created_at ASC, id ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, now.UTC(), MaxDeliveryAttempts))
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return Notification{}, false, err
		}
		return Notification{}, false, nil
	}
	if err != nil {
		return Notification{}, false, err
	}

	lockedUntil := now.UTC().Add(lease)
	tag, err := tx.Exec(ctx, `
		UPDATE notifications
		SET status = 'PROCESSING', delivery_attempts = delivery_attempts + 1, locked_until = $2, updated_at = $3
		WHERE id = $1
	`, notification.ID, lockedUntil, now.UTC())
	if err != nil {
		return Notification{}, false, err
	}
	if tag.RowsAffected() != 1 {
		return Notification{}, false, errors.New("notification delivery claim affected no rows")
	}
	notification.Status = StatusProcessing
	notification.DeliveryAttempts++
	notification.LockedUntil = &lockedUntil
	notification.UpdatedAt = now.UTC()
	if err := tx.Commit(ctx); err != nil {
		return Notification{}, false, err
	}
	return notification, true, nil
}

func (r *Repository) MarkDeliverySent(ctx context.Context, notification Notification, result ProviderResult, now time.Time) (Notification, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Notification{}, err
	}
	defer rollback(ctx, tx)
	claimLockedUntil := notification.LockedUntil
	notification.Status = StatusSent
	notification.LastError = ""
	notification.UpdatedAt = now.UTC()
	notification.LockedUntil = nil
	if err := updateDeliveryState(ctx, tx, notification, notification.NextAttemptAt, claimLockedUntil); err != nil {
		return Notification{}, err
	}
	delivery := Delivery{
		ID:                NewID(),
		NotificationID:    notification.ID,
		Provider:          "smtp",
		Status:            StatusSent,
		ProviderMessageID: result.ProviderMessageID,
		CreatedAt:         notification.UpdatedAt,
	}
	if err := insertDelivery(ctx, tx, delivery); err != nil {
		return Notification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Notification{}, err
	}
	return notification, nil
}

func (r *Repository) MarkDeliveryFailed(ctx context.Context, notification Notification, cause error, now time.Time) (Notification, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Notification{}, err
	}
	defer rollback(ctx, tx)
	claimLockedUntil := notification.LockedUntil
	notification.Status = StatusFailed
	notification.LastError = trimError(cause)
	notification.UpdatedAt = now.UTC()
	notification.LockedUntil = nil
	nextAttemptAt := now.UTC().Add(deliveryBackoff(notification.DeliveryAttempts))
	if notification.DeliveryAttempts >= MaxDeliveryAttempts {
		nextAttemptAt = now.UTC()
	}
	notification.NextAttemptAt = nextAttemptAt
	if err := updateDeliveryState(ctx, tx, notification, nextAttemptAt, claimLockedUntil); err != nil {
		return Notification{}, err
	}
	delivery := Delivery{
		ID:             NewID(),
		NotificationID: notification.ID,
		Provider:       "smtp",
		Status:         StatusFailed,
		Error:          notification.LastError,
		CreatedAt:      notification.UpdatedAt,
	}
	if err := insertDelivery(ctx, tx, delivery); err != nil {
		return Notification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Notification{}, err
	}
	return notification, nil
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
		SELECT id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, idempotency_key, delivery_attempts, next_attempt_at, locked_until, created_at, updated_at
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
		INSERT INTO notifications (id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, idempotency_key, delivery_attempts, next_attempt_at, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, notification.ID, notification.MessageID, notification.RecipientUserID, notification.RecipientEmail, notification.RoutingKey, notification.Subject, notification.Body, notification.Status, notification.LastError, notification.IdempotencyKey, notification.DeliveryAttempts, notification.NextAttemptAt, notification.LockedUntil, notification.CreatedAt, notification.UpdatedAt)
	return err
}

func updateDeliveryState(ctx context.Context, tx pgx.Tx, notification Notification, nextAttemptAt time.Time, claimLockedUntil *time.Time) error {
	tag, err := tx.Exec(ctx, `
		UPDATE notifications
		SET status = $2, last_error = $3, next_attempt_at = $4, locked_until = NULL, updated_at = $5
		WHERE id = $1
		  AND status = 'PROCESSING'
		  AND locked_until = $6
	`, notification.ID, notification.Status, notification.LastError, nextAttemptAt.UTC(), notification.UpdatedAt, claimLockedUntil)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleDeliveryClaim
	}
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
		SELECT id, message_id, recipient_user_id, recipient_email, routing_key, subject, body, status, last_error, idempotency_key, delivery_attempts, next_attempt_at, locked_until, created_at, updated_at
		FROM notifications
		WHERE message_id = $1
	`, messageID))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNotification(row scanner) (Notification, error) {
	var notification Notification
	err := row.Scan(&notification.ID, &notification.MessageID, &notification.RecipientUserID, &notification.RecipientEmail, &notification.RoutingKey, &notification.Subject, &notification.Body, &notification.Status, &notification.LastError, &notification.IdempotencyKey, &notification.DeliveryAttempts, &notification.NextAttemptAt, &notification.LockedUntil, &notification.CreatedAt, &notification.UpdatedAt)
	return notification, err
}

func deliveryBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return deliveryRetryBackoffs[0]
	}
	if attempts > len(deliveryRetryBackoffs) {
		return deliveryRetryBackoffs[len(deliveryRetryBackoffs)-1]
	}
	return deliveryRetryBackoffs[attempts-1]
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
