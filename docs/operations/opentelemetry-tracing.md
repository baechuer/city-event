# OpenTelemetry Tracing

## Current State

CityEvents currently propagates W3C `traceparent` and `tracestate` across HTTP
requests and RabbitMQ message headers. This gives trace continuity in logs and
message metadata, but it is not full OpenTelemetry span instrumentation yet.

The repository now includes `deploy/observability/otel-collector.yaml` so a
collector can be added without inventing observability topology later.

## Target Trace Path

A successful join workflow should eventually show:

1. API gateway HTTP span for the public request.
2. Event-registration HTTP span for capacity and join transaction handling.
3. Outbox relay span for publishing the domain event.
4. Feed or notification consumer span for eventual projection work.

## Acceptance Rubric

Manifest-ready tracing requires:

- W3C trace headers are accepted and propagated by HTTP middleware.
- RabbitMQ publish/consume preserves trace headers.
- Collector config exists and is documented.

Production-grade tracing requires:

- Go services use OpenTelemetry SDK spans, not only trace header propagation.
- OTLP exporter endpoint is configured per environment.
- Collector exports traces to a real backend such as Tempo, Jaeger, or an
  observability vendor.
- One join workflow trace is captured and linked from evidence docs.

## Claim Boundary

Safe now:

```text
Implemented trace-context propagation and added OpenTelemetry collector configuration for staging validation.
```

Not safe yet:

```text
Production distributed tracing is live.
```
