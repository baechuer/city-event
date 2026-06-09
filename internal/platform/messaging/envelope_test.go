package messaging

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEnvelopeValidatesAndMarshals(t *testing.T) {
	now := time.Date(2026, 6, 10, 1, 2, 3, 0, time.UTC)
	envelope, err := NewEnvelope("msg-1", "event.published", "event", "event-1", "corr-1", now, map[string]any{"eventId": "event-1"})
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	raw, err := envelope.MarshalJSONBytes()
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if decoded["messageId"] != "msg-1" || decoded["routingKey"] != "event.published" {
		t.Fatalf("unexpected envelope JSON: %s", string(raw))
	}
}

func TestNewEnvelopeRejectsInvalidInput(t *testing.T) {
	_, err := NewEnvelope("", "event.published", "event", "event-1", "", time.Now(), map[string]any{})
	if err == nil {
		t.Fatal("expected missing message id to fail")
	}
	_, err = NewEnvelope("msg-1", "event.published", "event", "event-1", "", time.Time{}, map[string]any{})
	if err == nil {
		t.Fatal("expected missing occurred at to fail")
	}
	_, err = NewEnvelope("msg-1", "event.published", "event", "event-1", "", time.Now(), nil)
	if err == nil {
		t.Fatal("expected nil payload to fail")
	}
}
