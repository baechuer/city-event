//go:build integration

package feedprojection

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProjectorDuplicateMessagesDoNotDuplicateEffects(t *testing.T) {
	ctx := context.Background()
	pool := setupFeedProjectionPostgres(t, ctx)
	projector := NewProjector(pool)
	eventID := "event-1"

	published := testEnvelope(t, "msg-event", "event.published", eventID, map[string]any{
		"eventId":  eventID,
		"title":    "Tech Meetup",
		"city":     "Sydney",
		"venue":    "Town Hall",
		"startsAt": "2026-07-01T09:00:00Z",
		"capacity": 10,
		"status":   "PUBLISHED",
	})
	if err := projector.HandleEnvelope(ctx, published); err != nil {
		t.Fatalf("project event: %v", err)
	}
	if err := projector.HandleEnvelope(ctx, published); err != nil {
		t.Fatalf("duplicate project event: %v", err)
	}

	confirmed := testEnvelope(t, "msg-join", "join.confirmed", "reg-1", map[string]any{
		"registrationId": "reg-1",
		"eventId":        eventID,
		"userId":         "user-1",
		"status":         "CONFIRMED",
	})
	if err := projector.HandleEnvelope(ctx, confirmed); err != nil {
		t.Fatalf("project join: %v", err)
	}
	if err := projector.HandleEnvelope(ctx, confirmed); err != nil {
		t.Fatalf("duplicate project join: %v", err)
	}

	canceled := testEnvelope(t, "msg-cancel", "join.canceled", "reg-1", map[string]any{
		"registrationId": "reg-1",
		"eventId":        eventID,
		"userId":         "user-1",
		"status":         "CANCELED",
		"previousStatus": "CONFIRMED",
	})
	if err := projector.HandleEnvelope(ctx, canceled); err != nil {
		t.Fatalf("project cancel: %v", err)
	}
	if err := projector.HandleEnvelope(ctx, canceled); err != nil {
		t.Fatalf("duplicate project cancel: %v", err)
	}

	promoted := testEnvelope(t, "msg-promote", "join.promoted", "reg-2", map[string]any{
		"registrationId": "reg-2",
		"eventId":        eventID,
		"userId":         "user-2",
		"status":         "CONFIRMED",
		"previousStatus": "WAITLISTED",
	})
	if err := projector.HandleEnvelope(ctx, promoted); err != nil {
		t.Fatalf("project promote: %v", err)
	}
	if err := projector.HandleEnvelope(ctx, promoted); err != nil {
		t.Fatalf("duplicate project promote: %v", err)
	}

	feedEvent, err := projector.GetEvent(ctx, eventID)
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
	if processed != 4 {
		t.Fatalf("processed count = %d, want 4", processed)
	}
}

func TestProjectorFailureDoesNotRecordProcessedMessage(t *testing.T) {
	ctx := context.Background()
	pool := setupFeedProjectionPostgres(t, ctx)
	projector := NewProjector(pool)

	bad := testEnvelope(t, "bad-msg", "event.published", "event-1", map[string]any{
		"eventId":  "event-1",
		"startsAt": "not-a-time",
	})
	if err := projector.HandleEnvelope(ctx, bad); err == nil {
		t.Fatal("expected bad event to fail")
	}
	processed, err := projector.ProcessedCount(ctx)
	if err != nil {
		t.Fatalf("processed count: %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed count = %d, want 0 after failed transaction", processed)
	}
}

func setupFeedProjectionPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
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
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS processed_messages;
		DROP TABLE IF EXISTS feed_events;
	`); err != nil {
		t.Fatalf("reset feed projection tables: %v", err)
	}
	path := filepath.Join("..", "..", "..", "migrations", "feed", "001_init.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return pool
}

func testEnvelope(t *testing.T, messageID, routingKey, aggregateID string, payload map[string]any) messaging.Envelope {
	t.Helper()
	envelope, err := messaging.NewEnvelope(messageID, routingKey, "test", aggregateID, "corr-1", time.Now().UTC(), payload)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	return envelope
}
