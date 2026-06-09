package feedprojection

import (
	"context"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/messaging"
)

func TestStringAndIntPayloadFields(t *testing.T) {
	payload := map[string]any{
		"eventId": " event-1 ",
		"count":   float64(3),
	}
	if got := stringField(payload, "eventId"); got != "event-1" {
		t.Fatalf("event id = %q", got)
	}
	if got := intField(payload, "count"); got != 3 {
		t.Fatalf("count = %d", got)
	}
}

func TestTimeFieldParsesRFC3339Nano(t *testing.T) {
	payload := map[string]any{"startsAt": "2026-07-01T09:00:00Z"}
	parsed, err := timeField(payload, "startsAt")
	if err != nil {
		t.Fatalf("time field: %v", err)
	}
	if parsed == nil || parsed.Format(time.RFC3339) != "2026-07-01T09:00:00Z" {
		t.Fatalf("parsed time = %v", parsed)
	}
}

func TestEnvelopeValidationBeforeProjection(t *testing.T) {
	projector := &Projector{}
	err := projector.HandleEnvelope(context.Background(), messaging.Envelope{})
	if err == nil {
		t.Fatal("expected invalid envelope to fail")
	}
}
