# Phase 13 CI/CD, Browser E2E, Rate Limiting, Observability, And Replica Readiness

## Objective

Phase 13 turns the existing local project evidence into repeatable gates:

- GitHub Actions validates tests, scripts, Docker Compose, phase gates, and image builds.
- Playwright runs a browser E2E flow against the real local stack.
- Shared HTTP middleware adds configurable endpoint rate limiting.
- Metrics are upgraded from basic request counters to method/path/status counters plus latency histograms.
- Kubernetes manifests include two replicas per workload plus PodDisruptionBudgets.
- Failure testing is represented by a static gate and a guarded live pod-deletion harness.

## Implementation Map

| Capability | Files |
| --- | --- |
| CI workflow | `.github/workflows/ci.yml` |
| Phase verifier | `scripts/verify-phase-13.sh` |
| Browser E2E | `frontend/playwright.config.mjs`, `frontend/e2e/cityevents.spec.mjs`, `frontend/package.json`, `frontend/package-lock.json` |
| Rate limiting | `internal/platform/httpapi/rate_limit.go`, `internal/platform/httpapi/router.go`, `internal/platform/config/config.go` |
| Observability metrics and trace context | `internal/platform/httpapi/observability.go`, `internal/platform/observability/observability.go`, `internal/platform/messaging/rabbitmq.go` |
| Kubernetes replicas/PDBs | `deploy/kubernetes/deployments.yaml`, `deploy/kubernetes/poddisruptionbudgets.yaml`, `deploy/kubernetes/kustomization.yaml` |
| Failure-test harness | `scripts/failure-test-kubernetes.sh`, `docs/testing/failure-testing.md` |

## CI/CD Design

The workflow has three jobs:

- `verify`: installs Go and Node, runs frontend unit tests, Go tests, Bash syntax checks, Docker Compose validation, and `scripts/verify-phase-13.sh`.
- `container-build`: builds every service image through the shared Dockerfile.
- `browser-e2e`: installs Chromium with Playwright and runs the browser E2E suite.

This is CI plus deployment-readiness build validation. It is not production CD yet because the repo does not push images to a registry or apply manifests to a live cluster.

## Browser E2E Scope

The Playwright test starts the local one-shot stack through `scripts/start-local.sh`, opens the frontend, verifies runtime API configuration, creates fresh test users, promotes one user through the seeded admin account, publishes an event through the browser UI, loads the direct live event detail route before relying on feed projection, signs in as another user, joins the event through the browser UI, and checks that `/metrics` exposes the new latency histogram.

This covers the gateway, auth, role update, event creation, feed projection, and join path from a browser-facing workflow. It does not replace lower-level concurrency tests or the CI-gated load evidence.

## Rate Limiting Design

Rate limiting is implemented once in the shared `httpapi` router so it applies consistently to all services that use `NewBaseRouter`.

Default scopes:

- auth writes: `RATE_LIMIT_AUTH_REQUESTS`
- other `POST`, `PATCH`, and `DELETE` requests: `RATE_LIMIT_MUTATION_REQUESTS`
- reads: `RATE_LIMIT_REQUESTS`

Health, readiness, metrics, root, and CORS preflight requests are excluded.

Current implementation: the middleware supports memory counters for tests and Redis-backed shared counters for local/Kubernetes runtime config. This is distributed across service replicas that share Redis, but production edge protection should still add ingress, gateway, or WAF controls.

## Observability Design

The metrics endpoint now exposes:

- `cityevents_http_requests_total{service,method,path,status}`
- `cityevents_http_request_duration_seconds_bucket`
- `cityevents_http_request_duration_seconds_sum`
- `cityevents_http_request_duration_seconds_count`
- `cityevents_rate_limited_requests_total{service,scope}`
- existing outbox and consumer counters

The path label uses the Chi route pattern when available to avoid a different metric series for every event ID.

HTTP middleware now accepts or creates W3C `traceparent`, preserves optional
`tracestate`, logs the trace context, and exposes it on the response. The
gateway forwards trace headers to auth and proxied services. RabbitMQ publishers
write trace headers to AMQP metadata, and consumers extract them into context.

This is trace-context propagation, not a full tracing backend. It is not yet
OpenTelemetry SDK spans, Prometheus scraping, Grafana dashboards, alerting, or
SLO monitoring.

## Kubernetes Replica Readiness

Every Deployment now declares `replicas: 2`, and every workload has a matching `PodDisruptionBudget` with `minAvailable: 1`.

This improves restart and voluntary-disruption readiness for stateless workloads. It still does not prove high availability because:

- the app has not been deployed and tested in a live cluster in this environment;
- Postgres, RabbitMQ, Redis, MinIO, and Mailpit remain single-instance local dependencies;
- no HorizontalPodAutoscaler is defined;
- live pod, worker, broker, cache, and database failure tests still need to be recorded.

## Verification

Static and unit checks:

```bash
go test ./...
(cd frontend && npm run verify)
bash ./scripts/verify-phase-13.sh
```

Full browser E2E:

```bash
bash ./scripts/verify-phase-13.sh --run-e2e
```

Kubernetes static failure-test readiness:

```bash
bash ./scripts/failure-test-kubernetes.sh
```

Live pod-deletion test, only inside the manual GitHub Actions `Heavy Evidence`
workflow:

```bash
bash ./scripts/failure-test-kubernetes.sh --live --deployment api-gateway
```

## Claim Boundary

Allowed:

```text
Added CI gates, browser E2E coverage, shared HTTP rate limiting, richer request metrics, replicated Kubernetes workload manifests, and a guarded pod-failure test harness.
```

Not allowed yet:

```text
Production CD, autoscaled production services, edge-grade DDoS protection, full observability stack, or failure-tested highly available Kubernetes deployment.
```
