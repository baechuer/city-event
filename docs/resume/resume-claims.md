# Resume Claims

## Primary Project Summary

CityEvents is a Go microservices platform for local event discovery and registration. It demonstrates gateway-mediated JWT authentication, rotating refresh-token sessions, role-based access control, a transactional core service, RabbitMQ-based asynchronous workflows, Redis-backed read caching, idempotent consumers, local media processing, basic observability, and Kubernetes-ready deployment manifests.

## Recommended Resume Bullets

Use these bullets as the strongest current version:

- Built a Go microservices event platform covering authentication, event publishing, event joining, waitlists, feed reads, notification records, and media processing.
- Added gateway JWT middleware with short-lived access tokens, rotating HttpOnly refresh tokens, `USER`/`ORGANIZER`/`ADMIN` roles, seeded admin bootstrapping, and role-gated event publishing/admin operations.
- Designed the event-registration service as the consistency boundary, using PostgreSQL transactions, row-level locking, unique constraints, and idempotent join behavior to prevent overbooking under tested concurrency.
- Implemented RabbitMQ-based asynchronous workflows with a transactional outbox, persistent messages, publisher confirms, retryable outbox failures, and idempotent consumers for feed and notification side effects.
- Added Redis-backed feed caching as a non-authoritative fast path with Postgres fallback, keeping the source of truth in durable storage.
- Added correlation IDs, structured logs, basic Prometheus-style metrics, and a debugging walkthrough to trace HTTP requests through outbox, RabbitMQ, feed projection, and notification records.
- Prepared services for Kubernetes deployment with Docker builds, Deployments, Services, ConfigMaps, Secret templates, health probes, resource limits, TLS ingress routing, and a documented high-availability roadmap.

## Short Version

Use this if space is limited:

```text
Built CityEvents, a Go microservices event platform with gateway JWT/RBAC, rotating refresh tokens, PostgreSQL-backed event registration, RabbitMQ asynchronous workflows, Redis feed caching, idempotent consumers, observability hooks, and Kubernetes-ready manifests.
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
The project is Kubernetes-ready, not highly available. The manifests include probes, resources, and config separation, but HA would require replicas, autoscaling policy, HA Postgres/RabbitMQ/Redis, and failure tests.
```

## Claims To Avoid

Do not say:

- exactly-once consumption
- messages are guaranteed to never be lost
- messages will always be stored
- highly available Kubernetes deployment
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
| Redis cache | feed cache tests and fallback tests |
| local notifications | Mailpit SMTP integration and provider failure tests |
| media processing | MinIO integration and worker failure tests |
| observability | correlation ID middleware, metrics endpoint, debugging walkthrough |
| Kubernetes readiness | `deploy/kubernetes/`, including ingress, `scripts/verify-phase-10.sh`, `scripts/verify-phase-11.sh` |

## Current Limitation Statement

Use this in interviews:

```text
The project is not production deployed and does not claim high availability. I treated those as evidence-gated claims: the next step would be a real cluster smoke test, replicated stateless services, HA dependencies, and failure testing.
```
