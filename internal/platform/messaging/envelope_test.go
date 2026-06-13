package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/observability"
	amqp "github.com/rabbitmq/amqp091-go"
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

func TestTraceHeadersFromContext(t *testing.T) {
	traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx := observability.ContextWithTraceContext(context.Background(), observability.TraceContext{
		TraceParent: traceParent,
		TraceState:  "vendor=value",
	})

	headers := traceHeadersFromContext(ctx)

	if headers[observability.TraceParentHeader] != traceParent {
		t.Fatalf("traceparent header = %#v", headers[observability.TraceParentHeader])
	}
	if headers[observability.TraceStateHeader] != "vendor=value" {
		t.Fatalf("tracestate header = %#v", headers[observability.TraceStateHeader])
	}
}

func TestContextWithAMQPTraceHeaders(t *testing.T) {
	traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx := ContextWithAMQPTraceHeaders(context.Background(), amqp.Table{
		observability.TraceParentHeader: traceParent,
		observability.TraceStateHeader:  "vendor=value",
	})

	trace := observability.TraceContextFromContext(ctx)
	if trace.TraceParent != traceParent || trace.TraceState != "vendor=value" {
		t.Fatalf("unexpected trace context: %+v", trace)
	}
}

func TestConsumerRetryDelaysIncrease(t *testing.T) {
	delays := ConsumerRetryDelays()
	if len(delays) != MaxConsumerRetries {
		t.Fatalf("retry delay count = %d, want %d", len(delays), MaxConsumerRetries)
	}
	for i := 1; i < len(delays); i++ {
		if delays[i] <= delays[i-1] {
			t.Fatalf("retry delay %d = %s, previous = %s", i, delays[i], delays[i-1])
		}
	}
	delays[0] = 0
	if ConsumerRetryDelays()[0] == 0 {
		t.Fatal("expected retry delay slice to be defensive copy")
	}
}

func TestDeliveryRetryCountParsesCommonHeaderTypes(t *testing.T) {
	cases := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{name: "nil", headers: nil, want: 0},
		{name: "int32", headers: amqp.Table{RetryCountHeader: int32(3)}, want: 3},
		{name: "int64", headers: amqp.Table{RetryCountHeader: int64(4)}, want: 4},
		{name: "string", headers: amqp.Table{RetryCountHeader: "5"}, want: 5},
		{name: "negative", headers: amqp.Table{RetryCountHeader: int32(-1)}, want: 0},
		{name: "invalid", headers: amqp.Table{RetryCountHeader: "bad"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeliveryRetryCount(tc.headers); got != tc.want {
				t.Fatalf("retry count = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDeliveryRoutingKeyPrefersOriginalRoutingHeader(t *testing.T) {
	delivery := amqp.Delivery{
		RoutingKey: FeedQueue,
		Headers: amqp.Table{
			OriginalRoutingHeader: "join.confirmed",
		},
	}
	if got := DeliveryRoutingKey(delivery); got != "join.confirmed" {
		t.Fatalf("routing key = %s, want join.confirmed", got)
	}

	delivery.Headers = nil
	if got := DeliveryRoutingKey(delivery); got != FeedQueue {
		t.Fatalf("routing key fallback = %s, want %s", got, FeedQueue)
	}
}
