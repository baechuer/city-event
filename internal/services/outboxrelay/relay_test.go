package outboxrelay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
)

type fakePublisher struct {
	err       error
	published []messaging.Envelope
}

func (p *fakePublisher) Publish(_ context.Context, _ string, envelope messaging.Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, envelope)
	return nil
}

func TestOutboxRecordEnvelopeUsesOutboxIDAsMessageID(t *testing.T) {
	record := OutboxRecord{
		ID:            "outbox-1",
		AggregateType: "event",
		AggregateID:   "event-1",
		RoutingKey:    "event.published",
		Payload:       map[string]any{"eventId": "event-1", "correlationId": "corr-1"},
		CreatedAt:     time.Now().UTC(),
	}
	envelope, err := record.Envelope()
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if envelope.MessageID != record.ID {
		t.Fatalf("message id = %s, want %s", envelope.MessageID, record.ID)
	}
	if envelope.CorrelationID != "corr-1" {
		t.Fatalf("correlation id = %s, want corr-1", envelope.CorrelationID)
	}
}

func TestOutboxRecordEnvelopeRejectsInvalidPayload(t *testing.T) {
	record := OutboxRecord{
		ID:            "outbox-1",
		AggregateType: "event",
		AggregateID:   "event-1",
		RoutingKey:    "event.published",
		CreatedAt:     time.Now().UTC(),
	}
	if _, err := record.Envelope(); err == nil {
		t.Fatal("expected nil payload to fail")
	}
}

func TestRetryBackoffIsBounded(t *testing.T) {
	if retryBackoff(1) <= 0 {
		t.Fatal("expected positive backoff")
	}
	if retryBackoff(100) != retryBackoff(6) {
		t.Fatal("expected high attempts to be capped")
	}
}

func TestFakePublisherRecordsMessages(t *testing.T) {
	publisher := &fakePublisher{}
	envelope, err := messaging.NewEnvelope("msg-1", "event.published", "event", "event-1", "", time.Now(), map[string]any{})
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	if err := publisher.Publish(context.Background(), envelope.RoutingKey, envelope); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("published count = %d, want 1", len(publisher.published))
	}
}

func TestFakePublisherReturnsError(t *testing.T) {
	want := errors.New("publish failed")
	publisher := &fakePublisher{err: want}
	envelope, err := messaging.NewEnvelope("msg-1", "event.published", "event", "event-1", "", time.Now(), map[string]any{})
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	if err := publisher.Publish(context.Background(), envelope.RoutingKey, envelope); !errors.Is(err, want) {
		t.Fatalf("publish error = %v, want %v", err, want)
	}
}
