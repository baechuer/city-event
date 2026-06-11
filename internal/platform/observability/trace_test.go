package observability

import (
	"context"
	"net/http"
	"testing"
)

const testTraceParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestTraceContextFromHeadersValidatesTraceParent(t *testing.T) {
	header := http.Header{}
	header.Set(TraceParentHeader, testTraceParent)
	header.Set(TraceStateHeader, "vendor=value")

	trace := TraceContextFromHeaders(header)
	if trace.TraceParent != testTraceParent || trace.TraceState != "vendor=value" {
		t.Fatalf("unexpected trace context: %+v", trace)
	}

	header.Set(TraceParentHeader, "invalid")
	if trace := TraceContextFromHeaders(header); trace.TraceParent != "" {
		t.Fatalf("invalid traceparent should be dropped: %+v", trace)
	}
}

func TestTraceContextInjection(t *testing.T) {
	ctx := ContextWithTraceContext(context.Background(), TraceContext{TraceParent: testTraceParent, TraceState: "vendor=value"})
	header := http.Header{}

	InjectTraceHeaders(header, ctx)

	if got := header.Get(TraceParentHeader); got != testTraceParent {
		t.Fatalf("traceparent = %q", got)
	}
	if got := header.Get(TraceStateHeader); got != "vendor=value" {
		t.Fatalf("tracestate = %q", got)
	}
}

func TestNewTraceParentIsValid(t *testing.T) {
	if traceParent := NewTraceParent(); !ValidTraceParent(traceParent) {
		t.Fatalf("generated traceparent is invalid: %s", traceParent)
	}
}
