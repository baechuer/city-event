package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracing middleware instruments HTTP requests with OpenTelemetry spans
func Tracing(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Use otelhttp middleware for automatic instrumentation
		return otelhttp.NewHandler(next, serviceName,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)
	}
}

// TracingTransport wraps an HTTP transport to propagate trace context
type TracingTransport struct {
	Base http.RoundTripper
}

// RoundTrip implements http.RoundTripper and injects trace context
func (t *TracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Start a client span
	tracer := otel.Tracer("bff-service")
	ctx, span := tracer.Start(ctx, "HTTP "+req.Method+" "+req.URL.Host+req.URL.Path,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	// Add attributes
	span.SetAttributes(
		attribute.String("http.method", req.Method),
		attribute.String("http.url", req.URL.String()),
		attribute.String("http.host", req.URL.Host),
	)

	// Inject trace context into outgoing request headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	// Make the request
	req = req.WithContext(ctx)
	resp, err := t.base().RoundTrip(req)

	if err != nil {
		span.RecordError(err)
		return resp, err
	}

	// Record response status
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	return resp, nil
}

func (t *TracingTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

// NewTracingHTTPClient creates an HTTP client that propagates trace context
func NewTracingHTTPClient() *http.Client {
	return &http.Client{
		Transport: &TracingTransport{
			Base: http.DefaultTransport,
		},
	}
}
