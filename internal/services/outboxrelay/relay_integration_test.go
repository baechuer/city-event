//go:build integration

package outboxrelay

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
	"github.com/baechuer/cityevents/internal/platform/observability"
	"github.com/baechuer/cityevents/internal/services/eventregistration"
	"github.com/baechuer/cityevents/internal/services/feedprojection"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRelayPublishesPersistentMessageAndMarksSent(t *testing.T) {
	ctx := observability.ContextWithCorrelationID(context.Background(), "corr-relay-test")
	pool, conn := setupPhase4Integration(t, ctx)

	eventSvc := eventregistration.NewService(eventregistration.NewPostgresRepository(pool))
	created := createRelayEvent(t, ctx, eventSvc, "organizer-1")

	publisher, err := messaging.NewConfirmingPublisher(conn)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })

	relay := NewRelay(NewStore(pool), publisher)
	published, err := relay.PublishBatch(ctx, 10)
	if err != nil {
		t.Fatalf("publish batch: %v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1", published)
	}

	delivery := getOneFeedDelivery(t, conn)
	if delivery.DeliveryMode != amqp.Persistent {
		t.Fatalf("delivery mode = %d, want persistent", delivery.DeliveryMode)
	}
	if delivery.RoutingKey != eventregistration.RoutingEventPublished {
		t.Fatalf("routing key = %s", delivery.RoutingKey)
	}
	if delivery.MessageId == "" {
		t.Fatal("expected AMQP message id")
	}
	if delivery.CorrelationId != "corr-relay-test" {
		t.Fatalf("delivery correlation id = %s", delivery.CorrelationId)
	}
	envelope, err := messaging.DecodeEnvelope(delivery.Body)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.MessageID == "" || envelope.AggregateID != created.Event.ID {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if envelope.CorrelationID != "corr-relay-test" {
		t.Fatalf("envelope correlation id = %s", envelope.CorrelationID)
	}
	_ = delivery.Ack(false)

	sent, err := countOutboxByStatus(ctx, pool, "SENT")
	if err != nil {
		t.Fatalf("sent count: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent count = %d, want 1", sent)
	}
}

func TestRelayToFeedProjectionSmoke(t *testing.T) {
	ctx := context.Background()
	pool, conn := setupPhase4Integration(t, ctx)

	eventSvc := eventregistration.NewService(eventregistration.NewPostgresRepository(pool))
	projector := feedprojection.NewProjector(pool)
	event := createRelayEvent(t, ctx, eventSvc, "organizer-1")
	if _, err := eventSvc.JoinEvent(ctx, event.Event.ID, "user-1", ""); err != nil {
		t.Fatalf("join event: %v", err)
	}

	publisher, err := messaging.NewConfirmingPublisher(conn)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })

	relay := NewRelay(NewStore(pool), publisher)
	published, err := relay.PublishBatch(ctx, 10)
	if err != nil {
		t.Fatalf("publish batch: %v", err)
	}
	if published != 2 {
		t.Fatalf("published = %d, want 2", published)
	}

	var joinEnvelope messaging.Envelope
	for i := 0; i < 2; i++ {
		delivery := getOneFeedDelivery(t, conn)
		envelope, err := messaging.DecodeEnvelope(delivery.Body)
		if err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if err := projector.HandleEnvelope(ctx, envelope); err != nil {
			t.Fatalf("project envelope %s: %v", envelope.RoutingKey, err)
		}
		if envelope.RoutingKey == eventregistration.RoutingJoinConfirmed {
			joinEnvelope = envelope
		}
		_ = delivery.Ack(false)
	}
	if joinEnvelope.MessageID == "" {
		t.Fatal("did not receive join.confirmed envelope")
	}
	if err := projector.HandleEnvelope(ctx, joinEnvelope); err != nil {
		t.Fatalf("duplicate project join: %v", err)
	}

	feedEvent, err := projector.GetEvent(ctx, event.Event.ID)
	if err != nil {
		t.Fatalf("get feed event: %v", err)
	}
	if feedEvent.ConfirmedCount != 1 {
		t.Fatalf("confirmed count = %d, want 1", feedEvent.ConfirmedCount)
	}
	processed, err := projector.ProcessedCount(ctx)
	if err != nil {
		t.Fatalf("processed count: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed count = %d, want 2 despite duplicate", processed)
	}
}

func TestRelayPublishesManyOutboxMessages(t *testing.T) {
	ctx := context.Background()
	pool, conn := setupPhase4Integration(t, ctx)
	eventSvc := eventregistration.NewService(eventregistration.NewPostgresRepository(pool))
	for i := 0; i < 100; i++ {
		createRelayEvent(t, ctx, eventSvc, fmt.Sprintf("organizer-%03d", i))
	}

	publisher, err := messaging.NewConfirmingPublisher(conn)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })

	relay := NewRelay(NewStore(pool), publisher)
	published, err := relay.PublishBatch(ctx, 100)
	if err != nil {
		t.Fatalf("publish batch: %v", err)
	}
	if published != 100 {
		t.Fatalf("published = %d, want 100", published)
	}
	sent, err := countOutboxByStatus(ctx, pool, "SENT")
	if err != nil {
		t.Fatalf("sent count: %v", err)
	}
	if sent != 100 {
		t.Fatalf("sent count = %d, want 100", sent)
	}
}

func TestTwoRelaysDoNotDoublePublishSameRow(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPhase4Integration(t, ctx)
	eventSvc := eventregistration.NewService(eventregistration.NewPostgresRepository(pool))
	createRelayEvent(t, ctx, eventSvc, "organizer-1")

	publisher := &countingPublisher{}
	relayA := NewRelay(NewStore(pool), publisher)
	relayB := NewRelay(NewStore(pool), publisher)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, relay := range []*Relay{relayA, relayB} {
		wg.Add(1)
		go func(r *Relay) {
			defer wg.Done()
			_, err := r.PublishBatch(ctx, 1)
			errs <- err
		}(relay)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("relay error: %v", err)
		}
	}
	if publisher.Count() != 1 {
		t.Fatalf("publisher count = %d, want 1", publisher.Count())
	}
}

func TestRelayMarksFailedPublishRetryable(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPhase4Integration(t, ctx)
	eventSvc := eventregistration.NewService(eventregistration.NewPostgresRepository(pool))
	createRelayEvent(t, ctx, eventSvc, "organizer-1")

	relay := NewRelay(NewStore(pool), &failingPublisher{err: errors.New("rabbit down")})
	published, err := relay.PublishBatch(ctx, 1)
	if err == nil {
		t.Fatal("expected publish batch to fail")
	}
	if published != 0 {
		t.Fatalf("published = %d, want 0", published)
	}

	var status, lastError string
	var attempts int
	var availableAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT status, attempts, available_at, last_error
		FROM outbox_messages
		LIMIT 1
	`).Scan(&status, &attempts, &availableAt, &lastError); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if status != "FAILED" || attempts != 1 || lastError == "" || !availableAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected retry state status=%s attempts=%d availableAt=%s lastError=%q", status, attempts, availableAt, lastError)
	}
}

type countingPublisher struct {
	mu    sync.Mutex
	count int
}

func (p *countingPublisher) Publish(context.Context, string, messaging.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	return nil
}

func (p *countingPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

type failingPublisher struct {
	err error
}

func (p *failingPublisher) Publish(context.Context, string, messaging.Envelope) error {
	return p.err
}

func setupPhase4Integration(t *testing.T, ctx context.Context) (*pgxpool.Pool, *amqp.Connection) {
	t.Helper()
	dsn := os.Getenv("PHASE4_TEST_DATABASE_URL")
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
	applyPhase4Migrations(t, ctx, pool)

	rabbitURL := os.Getenv("PHASE4_TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://cityevents:cityevents@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("connect rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	resetRabbitTopology(t, conn)
	return pool, conn
}

func applyPhase4Migrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS processed_messages;
		DROP TABLE IF EXISTS feed_events;
		DROP TABLE IF EXISTS outbox_messages;
		DROP TABLE IF EXISTS event_registrations;
		DROP TABLE IF EXISTS events;
	`); err != nil {
		t.Fatalf("reset phase 4 tables: %v", err)
	}
	for _, path := range []string{
		filepath.Join("..", "..", "..", "migrations", "eventregistration", "001_init.sql"),
		filepath.Join("..", "..", "..", "migrations", "eventregistration", "002_outbox_relay.sql"),
		filepath.Join("..", "..", "..", "migrations", "feed", "001_init.sql"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
}

func resetRabbitTopology(t *testing.T, conn *amqp.Connection) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("rabbit channel: %v", err)
	}
	defer ch.Close()
	if err := messaging.DeclareTopology(ch); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
	if _, err := ch.QueuePurge(messaging.FeedQueue, false); err != nil {
		t.Fatalf("purge feed queue: %v", err)
	}
	if _, err := ch.QueuePurge(messaging.FeedDeadLetterQueue, false); err != nil {
		t.Fatalf("purge feed dlq: %v", err)
	}
	if _, err := ch.QueuePurge(messaging.NotificationQueue, false); err != nil {
		t.Fatalf("purge notification queue: %v", err)
	}
	if _, err := ch.QueuePurge(messaging.NotificationDeadLetterQueue, false); err != nil {
		t.Fatalf("purge notification dlq: %v", err)
	}
}

func createRelayEvent(t *testing.T, ctx context.Context, svc *eventregistration.Service, organizerID string) eventregistration.EventDetail {
	t.Helper()
	event, err := svc.CreateEvent(ctx, eventregistration.CreateEventCommand{
		OrganizerID: organizerID,
		Title:       "Phase 4 Event",
		Description: "RabbitMQ relay test",
		City:        "Sydney",
		Venue:       "Town Hall",
		StartsAt:    time.Now().UTC().Add(24 * time.Hour),
		Capacity:    10,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event
}

func getOneFeedDelivery(t *testing.T, conn *amqp.Connection) amqp.Delivery {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("rabbit channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		delivery, ok, err := ch.Get(messaging.FeedQueue, false)
		if err != nil {
			t.Fatalf("get feed delivery: %v", err)
		}
		if ok {
			return delivery
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for feed delivery")
	return amqp.Delivery{}
}

func countOutboxByStatus(ctx context.Context, pool *pgxpool.Pool, status string) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_messages WHERE status = $1`, status).Scan(&count)
	return count, err
}
