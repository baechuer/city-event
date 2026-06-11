# Resume Claims

## Primary Project Summary

CityEvents is a Go microservices platform for local event discovery and registration. It demonstrates gateway-mediated JWT authentication, rotating refresh-token sessions, CSRF-protected cookie refresh, role-based access control, a transactional core service, RabbitMQ-based asynchronous workflows, Redis-backed caching, idempotent consumers, local media processing, shared HTTP rate limiting, Prometheus-style request metrics, Playwright browser E2E coverage, GitHub Actions CI gates, Kubernetes-ready replicated deployment manifests, and local Kubernetes smoke-test tooling.

## Recommended Resume Bullets

Use these bullets as the strongest current version:

- Built a Go microservices event platform covering authentication, event publishing, event joining, waitlists, feed reads, notification records, and media processing.
- Added gateway JWT middleware with short-lived access tokens, rotating HttpOnly refresh tokens, double-submit CSRF protection for cookie refresh/logout, `USER`/`ORGANIZER`/`ADMIN` roles, seeded admin bootstrapping, and role-gated event publishing/admin operations.
- Designed the event-registration service as the consistency boundary, using PostgreSQL transactions, row-level locking, unique constraints, and idempotent join behavior to prevent overbooking under tested concurrency.
- Implemented RabbitMQ-based asynchronous workflows with a transactional outbox, persistent messages, publisher confirms, retryable outbox failures, and idempotent consumers for feed and notification side effects.
- Added Redis-backed feed caching and access-token revocation caching as non-authoritative fast paths with durable Postgres fallback.
- Added correlation IDs, structured logs, Prometheus-style request counters, latency histograms, rate-limit counters, and a debugging walkthrough to trace HTTP requests through outbox, RabbitMQ, feed projection, and notification records.
- Added shared HTTP rate limiting for auth, mutation, and read endpoints with tested 429 responses and explicit per-pod/distributed-limit caveats.
- Added GitHub Actions gates and a Playwright browser E2E flow covering organizer publishing and attendee joining against the local stack.
- Prepared services for Kubernetes deployment with Docker builds, Deployments, Services, ConfigMaps, Secret templates, health probes, resource limits, two replicas per workload, PodDisruptionBudgets, forced HTTPS ingress routing, a cert-manager certificate example, a local Minikube smoke/failure runner, and a documented high-availability roadmap.

## Short Version

Use this if space is limited:

```text
Built CityEvents, a Go microservices event platform with gateway JWT/RBAC, rotating refresh tokens, CSRF-protected cookie refresh, PostgreSQL-backed event registration, RabbitMQ asynchronous workflows, Redis caching, idempotent consumers, rate limiting, request metrics, Playwright E2E coverage, CI gates, Kubernetes-ready replicated manifests, and local Kubernetes smoke-test tooling.
```

## Interview Framing

Lead with the join workflow:

```text
The critical path is event joining. I made event registration the consistency boundary because capacity, waitlist, cancellation, and promotion all need the same transaction boundary. Postgres owns truth; RabbitMQ and Redis are downstream support systems.
```

Explain messaging precisely:

```text
I do not claim exactly-once RabbitMQ consumption. RabbitMQ gives at-least-once delivery, so the application uses a transactional outbox for publishing and durable idempotency checks in consumers to get exactly-once business effects where the tests prove it.
```

Explain Kubernetes honestly:

```text
The project is Kubernetes-ready, not highly available. The manifests include replicas, PodDisruptionBudgets, probes, resources, config separation, and local smoke-test tooling, but HA would still require production-grade failure evidence, autoscaling policy, HA Postgres/RabbitMQ/Redis, and dependency failure tests.
```

Current evidence boundary:

```text
The local Kubernetes smoke runner exists and static verification passes, but the 2026-06-11 live Minikube attempt failed before app deployment because the Minikube apiserver did not start. Do not claim live Kubernetes recovery until that run passes.
```

## Claims To Avoid

Do not say:

- exactly-once consumption
- messages are guaranteed to never be lost
- messages will always be stored
- highly available Kubernetes deployment
- distributed rate limiting
- production deployed
- autoscaled production microservices
- full OpenTelemetry/Grafana observability

## Evidence Map

| Resume claim | Evidence |
| --- | --- |
| microservices | `cmd/`, `internal/services/`, Dockerfile, Kubernetes manifests |
| gateway JWT/RBAC | `internal/services/gateway/`, `internal/services/auth/`, role-gated event-registration tests |
| event registration correctness | event-registration unit and integration tests |
| no overbooking under tested concurrency | concurrent event-registration integration tests |
| async decoupling | outbox relay, RabbitMQ topology, feed and notification consumers |
| idempotent business effects | processed-message tables and duplicate-message tests |
| Redis cache | feed cache tests, revocation-cache decorator tests, and fallback tests |
| local notifications | Mailpit SMTP integration and provider failure tests |
| media processing | MinIO integration and worker failure tests |
| rate limiting | `internal/platform/httpapi/rate_limit.go`, router tests, `docs/security/rate-limiting.md` |
| browser E2E | `frontend/e2e/cityevents.spec.mjs`, `docs/testing/browser-e2e.md` |
| CI gates | `.github/workflows/ci.yml`, `scripts/verify-phase-14.sh` |
| observability | correlation ID middleware, metrics endpoint, request histograms, debugging walkthrough |
| Kubernetes readiness | `deploy/kubernetes/`, including ingress, replicas, PodDisruptionBudgets, local overlay, `scripts/verify-phase-10.sh`, `scripts/verify-phase-11.sh`, `scripts/failure-test-kubernetes.sh`, `scripts/k8s-live-smoke.sh` |
| local load evidence | `scripts/load-test-local.sh`, `docs/testing/load-testing.md`, `tmp/load-test-local/<run-id>/summary.md` after each run |

## Current Limitation Statement

Use this in interviews:

```text
The project is not production deployed and does not claim high availability. I treated those as evidence-gated claims: the next step would be a real cluster smoke test under failure, HA dependencies, autoscaling policy, and recorded dependency failure testing.
```

Local load-test wording:

```text
I added a repeatable local gateway-level load test that registers users, creates an event, performs concurrent joins, and verifies capacity/waitlist invariants plus eventual feed projection. I do not treat that as a production throughput benchmark.
```
