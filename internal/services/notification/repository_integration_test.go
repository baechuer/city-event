//go:build integration

package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestNotificationRepositoryIdempotentSuccess(t *testing.T) {
	ctx := context.Background()
	pool := setupNotificationPostgres(t, ctx)
	repo := NewRepository(pool)
	provider := &fakeProvider{}
	envelope := testNotificationEnvelope(t, "msg-1", "join.confirmed", map[string]any{"userId": "user-1", "eventId": "event-1"})

	first, err := repo.ProcessEnvelope(ctx, envelope, provider)
	if err != nil {
		t.Fatalf("process first: %v", err)
	}
	if first.Notification.Status != StatusSent {
		t.Fatalf("status = %s, want sent", first.Notification.Status)
	}
	for i := 0; i < 10; i++ {
		result, err := repo.ProcessEnvelope(ctx, envelope, provider)
		if err != nil {
			t.Fatalf("duplicate process %d: %v", i, err)
		}
		if !result.Duplicate {
			t.Fatalf("duplicate process %d was not marked duplicate", i)
		}
	}
	assertNotificationCounts(t, ctx, repo, 1, 1, 1)
	if provider.Count() != 1 {
		t.Fatalf("provider sends = %d, want 1", provider.Count())
	}
}

func TestNotificationRepositoryProviderFailureRecordsFailed(t *testing.T) {
	ctx := context.Background()
	pool := setupNotificationPostgres(t, ctx)
	repo := NewRepository(pool)
	provider := &fakeProvider{err: errors.New("smtp down")}
	envelope := testNotificationEnvelope(t, "msg-1", "join.waitlisted", map[string]any{"userId": "user-1", "eventId": "event-1"})

	result, err := repo.ProcessEnvelope(ctx, envelope, provider)
	if err != nil {
		t.Fatalf("process envelope: %v", err)
	}
	if result.Notification.Status != StatusFailed || result.Notification.LastError == "" {
		t.Fatalf("notification = %+v, want failed with error", result.Notification)
	}
	assertNotificationCounts(t, ctx, repo, 1, 1, 1)
}

func TestNotificationRepositoryHundredMessages(t *testing.T) {
	ctx := context.Background()
	pool := setupNotificationPostgres(t, ctx)
	repo := NewRepository(pool)
	provider := &fakeProvider{}
	for i := 0; i < 100; i++ {
		envelope := testNotificationEnvelope(t, fmt.Sprintf("msg-%03d", i), "join.promoted", map[string]any{
			"userId":  fmt.Sprintf("user-%03d", i),
			"eventId": "event-1",
		})
		if _, err := repo.ProcessEnvelope(ctx, envelope, provider); err != nil {
			t.Fatalf("process %d: %v", i, err)
		}
	}
	assertNotificationCounts(t, ctx, repo, 100, 100, 100)
}

func TestNotificationRabbitMQConsumePath(t *testing.T) {
	ctx := context.Background()
	pool := setupNotificationPostgres(t, ctx)
	conn := setupNotificationRabbit(t)
	repo := NewRepository(pool)
	provider := &fakeProvider{}

	publisher, err := messaging.NewConfirmingPublisher(conn)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	envelope := testNotificationEnvelope(t, "msg-rabbit", "join.confirmed", map[string]any{"userId": "user-1", "eventId": "event-1"})
	if err := publisher.Publish(ctx, envelope.RoutingKey, envelope); err != nil {
		t.Fatalf("publish: %v", err)
	}

	delivery := getNotificationDelivery(t, conn)
	if delivery.RoutingKey != "join.confirmed" {
		t.Fatalf("routing key = %s", delivery.RoutingKey)
	}
	decoded, err := messaging.DecodeEnvelope(delivery.Body)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if _, err := repo.ProcessEnvelope(ctx, decoded, provider); err != nil {
		t.Fatalf("process delivery: %v", err)
	}
	_ = delivery.Ack(false)
	assertNotificationCounts(t, ctx, repo, 1, 1, 1)
}

func TestSMTPProviderAcceptsMailpitDelivery(t *testing.T) {
	addr := os.Getenv("PHASE6_TEST_SMTP_ADDR")
	if addr == "" {
		addr = "localhost:1025"
	}
	provider := NewSMTPProvider(addr)
	_, err := provider.Send(context.Background(), Message{
		ID:      "smtp-test",
		To:      "user-1@cityevents.local",
		Subject: "Phase 6 SMTP smoke",
		Body:    "This message verifies local Mailpit SMTP acceptance.",
	})
	if err != nil {
		t.Fatalf("send mailpit message: %v", err)
	}
}

type fakeProvider struct {
	mu    sync.Mutex
	err   error
	sends []Message
}

func (p *fakeProvider) Send(_ context.Context, msg Message) (ProviderResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sends = append(p.sends, msg)
	if p.err != nil {
		return ProviderResult{}, p.err
	}
	return ProviderResult{ProviderMessageID: msg.ID + "-fake"}, nil
}

func (p *fakeProvider) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sends)
}

func setupNotificationPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PHASE6_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_lock(424242)`); err != nil {
		t.Fatalf("take integration schema lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock(424242)`)
	})
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS notification_deliveries;
		DROP TABLE IF EXISTS notification_processed_messages;
		DROP TABLE IF EXISTS notifications;
	`); err != nil {
		t.Fatalf("reset notification tables: %v", err)
	}
	path := filepath.Join("..", "..", "..", "migrations", "notification", "001_init.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return pool
}

func setupNotificationRabbit(t *testing.T) *amqp.Connection {
	t.Helper()
	rabbitURL := os.Getenv("PHASE6_TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://cityevents:cityevents@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("connect rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("rabbit channel: %v", err)
	}
	defer ch.Close()
	if err := messaging.DeclareTopology(ch); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
	for _, queue := range []string{messaging.NotificationQueue, messaging.NotificationDeadLetterQueue} {
		if _, err := ch.QueuePurge(queue, false); err != nil {
			t.Fatalf("purge %s: %v", queue, err)
		}
	}
	return conn
}

func getNotificationDelivery(t *testing.T, conn *amqp.Connection) amqp.Delivery {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("rabbit channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		delivery, ok, err := ch.Get(messaging.NotificationQueue, false)
		if err != nil {
			t.Fatalf("get notification delivery: %v", err)
		}
		if ok {
			return delivery
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for notification delivery")
	return amqp.Delivery{}
}

func assertNotificationCounts(t *testing.T, ctx context.Context, repo *Repository, notifications, deliveries, processed int) {
	t.Helper()
	gotNotifications, err := repo.NotificationCount(ctx)
	if err != nil {
		t.Fatalf("notification count: %v", err)
	}
	gotDeliveries, err := repo.DeliveryCount(ctx)
	if err != nil {
		t.Fatalf("delivery count: %v", err)
	}
	gotProcessed, err := repo.ProcessedCount(ctx)
	if err != nil {
		t.Fatalf("processed count: %v", err)
	}
	if gotNotifications != notifications || gotDeliveries != deliveries || gotProcessed != processed {
		t.Fatalf("counts notifications=%d deliveries=%d processed=%d, want %d/%d/%d", gotNotifications, gotDeliveries, gotProcessed, notifications, deliveries, processed)
	}
}
