# Final Evidence Audit

## Purpose

This audit maps CityEvents resume-level claims to repository evidence.

The project is resume-ready only when claims are phrased at the same strength as the implementation and verification evidence. This document is intentionally conservative.

## System Summary

CityEvents is a Go microservices event platform for local city events. Users can register, log in, create events, browse events, join events, be waitlisted, cancel joins, receive local notification records, and attach media.

The rebuilt architecture uses:

- Go HTTP services with Chi routing
- Postgres as the source of truth
- RabbitMQ for asynchronous projections and side effects
- Redis as a non-authoritative feed cache and token-revocation acceleration cache
- MinIO for local S3-compatible media storage
- Mailpit for local email delivery evidence
- Docker Compose for local dependencies
- Kubernetes manifests for deployment readiness

## Evidence Matrix

| Claim | Status | Evidence |
| --- | --- | --- |
| Go microservices architecture | Supported | service commands under `cmd/`, shared platform packages under `internal/platform`, separate service packages under `internal/services`, Dockerfile, Kubernetes manifests |
| Product workflow | Supported locally | auth, event registration, feed, notification, media, and frontend tests; Phase 2 to Phase 8 verification scripts |
| Strong event-registration consistency | Supported | `event-registration-service` owns event creation, capacity, waitlist, cancellation, promotion, and outbox writes; concurrent join and cancellation integration tests |
| No overbooking under tested concurrency | Supported | `TestPostgresConcurrentJoinCapacityInvariant`, `TestPostgresConcurrentSameUserJoinCreatesOneActiveRegistration`, `TestPostgresConcurrentCancelDoesNotOverPromote` |
| Asynchronous decoupling | Supported | transactional outbox, outbox relay, RabbitMQ topology, feed projection, notification consumer |
| Reliable messaging semantics | Supported with precise wording | persistent RabbitMQ publishing, publisher confirms, retryable outbox failure state, durable consumer dedupe tables |
| Exactly-once consumption | Not supported | RabbitMQ and the app use at-least-once delivery plus idempotent business effects |
| Redis caching | Supported | feed service cache tests and fallback behavior; Redis is not source of truth |
| Auth browser hardening | Supported with precise wording | short access-token TTL, in-memory frontend access token, rotating HttpOnly refresh cookie, double-submit CSRF token for refresh/logout, explicit credentialed CORS |
| Access-token revocation cache | Supported | Postgres remains source of truth; Redis decorator caches revoked JWT IDs and falls back to Postgres on cache outage |
| Notification side effects | Supported locally | notification decision tests, idempotent repository tests, provider failure tests, Mailpit SMTP integration |
| Media worker | Supported locally | metadata repository tests, MinIO integration, worker state transition tests, failure-state tests |
| Observability/debugging | Supported as basic observability | correlation IDs, structured request logs, `/metrics`, outbox correlation propagation, debugging walkthrough |
| Kubernetes readiness | Supported | Dockerfile, Kubernetes Deployments/Services/ConfigMap/Secret template/Ingress/probes/resource limits, `scripts/verify-phase-10.sh` |
| High availability | Deferred | `docs/architecture/high-availability-decision.md`; manifests are single-replica and dependencies are not HA |
| Production deployment | Not supported | no verified live cluster run, managed secrets, production database, or failure-test evidence; TLS is manifest/example coverage only |

## Verification Commands

Minimum local verification:

```bash
go test ./...
docker compose config --quiet
./scripts/verify-phase-10.sh
./scripts/verify-phase-11.sh
./scripts/verify-phase-12.sh
```

Full local integration verification when Docker dependencies are available:

```bash
./scripts/verify-phase-9.sh
./scripts/verify-phase-12.sh --run-full-integration
```

## Current Safe Claims

Safe:

```text
Built CityEvents, a Go microservices event platform with authentication, event publishing, event joining, waitlists, feed reads, notification records, and media processing.
```

Safe:

```text
Implemented a concurrency-safe event-registration service using PostgreSQL transactions, row-level locking, unique constraints, idempotent join behavior, waitlist promotion, and integration tests that verify capacity is not exceeded.
```

Safe:

```text
Implemented RabbitMQ-based asynchronous workflows with a transactional outbox, persistent publishing, publisher confirms, retryable failure state, and idempotent consumers for feed and notification side effects.
```

Safe:

```text
Added Redis-backed feed caching and access-token revocation caching as non-authoritative fast paths with Postgres fallback.
```

Safe:

```text
Implemented browser auth hardening with memory-only access tokens, rotating HttpOnly refresh tokens, double-submit CSRF protection for refresh/logout, explicit credentialed CORS, and Redis-assisted revocation checks backed by durable Postgres records.
```

Safe:

```text
Added correlation IDs, structured request logging, basic Prometheus-style metrics, and a debugging walkthrough for tracing distributed workflows.
```

Safe:

```text
Prepared the services for Kubernetes deployment with container builds, Deployments, Services, health probes, configuration separation, Secret templates, resource limits, forced HTTPS ingress routing, a cert-manager certificate example, and documented high-availability requirements.
```

## Claims To Avoid

Avoid:

```text
Exactly-once RabbitMQ consumption.
```

Avoid:

```text
Guaranteed no message loss.
```

Avoid:

```text
Messages will always be stored.
```

Avoid:

```text
Highly available Kubernetes deployment.
```

Avoid:

```text
Production-deployed distributed system.
```

Avoid:

```text
Autoscaled production microservices.
```

## Remaining Gaps

Highest priority gaps before stronger claims:

- run and record a real Kubernetes deployment smoke test in Kind, Minikube, or a cloud cluster
- add production-grade dependency HA: managed Postgres, RabbitMQ quorum queues or cluster, managed Redis or Redis Cluster
- add failure tests for pod deletion, worker restart, RabbitMQ restart, Redis outage, and Postgres outage
- run a real TLS ingress smoke test with a valid `cityevents-tls` secret or cert-manager-issued certificate
- add CI jobs for integration tests with service containers
- add OpenTelemetry collector, trace backend, Prometheus scraping, and Grafana dashboard if claiming production observability
- replace the dependency-free frontend with the intended React/TypeScript/Vite frontend if the frontend is meant to be a primary claim

## Final Verdict

CityEvents is resume-ready as a portfolio-grade distributed systems project if the resume wording stays precise.

The strongest story is:

```text
correct transactional core + at-least-once messaging + idempotent business effects + Redis caching + browser auth hardening + observability + Kubernetes readiness
```

The project is not yet a production HA system.
