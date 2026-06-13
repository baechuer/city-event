# Alerts And Dashboards

## Purpose

This file records the current alerting and dashboard contract for CityEvents.
It keeps observability claims evidence-bounded: the repository now has
Prometheus/Grafana configuration, but a production observability stack is only
proven after these files are applied to a real cluster and alert delivery is
tested.

## Files

| File | Purpose |
| --- | --- |
| `deploy/observability/prometheus-rules.yaml` | Prometheus Operator alert rules for async retries, DLQs, outbox `DEAD`, and rate-limit store errors. |
| `deploy/observability/grafana/cityevents-async-ops-dashboard.json` | Grafana dashboard for consumer outcomes, outbox outcomes, DLQ events, HTTP p95 latency, and RabbitMQ DLQ depth. |
| `deploy/observability/otel-collector.yaml` | Minimal OpenTelemetry Collector config for OTLP traces/metrics and debug export during staging validation. |
| `docs/operations/async-failure-runbook.md` | Human recovery steps for DLQ copy-back and outbox requeue. |

## Acceptance Rubric

To count as implemented:

- Alert rules exist for consumer DLQ events, outbox terminal `DEAD` events,
  retry spikes, and Redis rate-limit store errors.
- Dashboard panels exist for consumer states, outbox states, DLQ events, HTTP
  latency, and exporter-backed RabbitMQ DLQ depth.
- Docs state which panels depend only on app metrics and which depend on
  external exporters.
- Static verification checks the files so future edits do not drift.

To count as production evidence:

- Prometheus scrapes `/metrics` from the gateway.
- RabbitMQ exporter is installed if queue-depth panels or alerts are claimed.
- Alertmanager routes a test alert to the intended destination.
- A GitHub Actions heavy-evidence artifact or staging run records screenshots,
  alert firing/resolution, and the commit SHA.

## Claim Boundary

Safe now:

```text
Added Prometheus alert rules and Grafana dashboard configuration for async retry, DLQ, outbox, rate-limit, and latency signals.
```

Not safe yet:

```text
Production alerting is live.
```
