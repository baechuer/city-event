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
- GitHub Actions for CI/build gates
- Playwright for browser E2E testing
- Kubernetes manifests and a local overlay for replicated deployment readiness

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
| Rate limiting | Supported locally with caveat | shared fixed-window HTTP middleware, config/env controls, 429/Retry-After tests, rate-limit metric; current limiter is per process, not distributed |
| Notification side effects | Supported locally | notification decision tests, idempotent repository tests, provider failure tests, Mailpit SMTP integration |
| Media worker | Supported locally | metadata repository tests, MinIO integration, worker state transition tests, failure-state tests |
| Browser E2E | Supported locally | Playwright organizer publish and attendee join flow against the local stack |
| CI gates | Supported | GitHub Actions workflow for Go tests, frontend tests, phase verification, service image builds, and browser E2E |
| Observability/debugging | Supported as basic observability | correlation IDs, structured request logs, `/metrics`, request counters, latency histograms, rate-limit counters, outbox correlation propagation, debugging walkthrough |
| Local load/correctness smoke | Supported locally | `scripts/load-test-local.sh` verifies gateway-level auth, event creation, concurrent joins, capacity/waitlist invariant, and feed projection |
| Kubernetes readiness | Supported | Dockerfile, Kubernetes Deployments/Services/ConfigMap/Secret template/Ingress/probes/resource limits/replicas/PDBs, local overlay, `scripts/verify-phase-10.sh`, `scripts/verify-phase-13.sh`, `scripts/verify-phase-14.sh` |
| Local Kubernetes smoke | Tooling supported; live run blocked locally | `scripts/k8s-live-smoke.sh --start-minikube --run-failure`; 2026-06-11 attempt failed before app deployment because Minikube apiserver did not start |
| High availability | Deferred | `docs/architecture/high-availability-decision.md`; manifests have replicas/PDBs and local smoke tooling, but production HA dependencies, multi-node evidence, continuous traffic failure tests, and autoscaling are not present |
| Production deployment | Not supported | no verified live cluster run, managed secrets, production database, or failure-test evidence; TLS is manifest/example coverage only |

## Verification Commands

Minimum local verification:

```bash
go test ./...
docker compose config --quiet
./scripts/verify-phase-10.sh
./scripts/verify-phase-11.sh
./scripts/verify-phase-12.sh
./scripts/verify-phase-13.sh
./scripts/verify-phase-14.sh
```

Full local integration verification when Docker dependencies are available:

```bash
./scripts/verify-phase-9.sh
./scripts/verify-phase-12.sh --run-full-integration
./scripts/load-test-local.sh --start-stack --users 80 --capacity 25 --concurrency 20
./scripts/verify-phase-13.sh --run-e2e
# after Minikube is healthy:
./scripts/k8s-live-smoke.sh --start-minikube --run-failure
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
Implemented browser auth hardening with memory-only access tokens, rotating HttpOnly refresh tokens, double-submit CSRF protection for refresh/logout, explicit credentialed CORS, shared HTTP rate limiting, and Redis-assisted revocation checks backed by durable Postgres records.
```

Safe:

```text
Added correlation IDs, structured request logging, Prometheus-style request counters, latency histograms, rate-limit counters, and a debugging walkthrough for tracing distributed workflows.
```

Safe:

```text
Added GitHub Actions gates and Playwright browser E2E coverage for the organizer publish and attendee join workflow.
```

Safe:

```text
Added a repeatable local gateway-level load test that verifies concurrent event joins preserve capacity and waitlist invariants while feed projection catches up eventually.
```

Safe:

```text
Prepared the services for Kubernetes deployment with container builds, Deployments, Services, health probes, configuration separation, Secret templates, resource limits, two replicas per workload, PodDisruptionBudgets, forced HTTPS ingress routing, a cert-manager certificate example, a local Minikube smoke-test overlay, and documented high-availability requirements.
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

Avoid:

```text
Distributed rate limiting.
```

## Remaining Gaps

Highest priority gaps before stronger claims:

- repair or recreate the local Minikube profile, then run and record a real Kubernetes deployment smoke test in Kind, Minikube, or a cloud cluster
- add continuous traffic during Kubernetes pod deletion, not only post-replacement readiness
- add production-grade dependency HA: managed Postgres, RabbitMQ quorum queues or cluster, managed Redis or Redis Cluster
- add failure tests for pod deletion, worker restart, RabbitMQ restart, Redis outage, and Postgres outage
- run a real TLS ingress smoke test with a valid `cityevents-tls` secret or cert-manager-issued certificate
- repeat load tests in a controlled environment with resource metrics before making throughput claims
- add registry push and controlled deployment jobs after secrets and cluster target are available
- replace per-pod rate limiting with Redis, ingress, or gateway-level distributed rate limiting before claiming distributed abuse protection
- add OpenTelemetry collector, trace backend, Prometheus scraping, and Grafana dashboard if claiming production observability
- replace the plain JavaScript frontend with the intended React/TypeScript/Vite frontend if the frontend is meant to be a primary claim

## Final Verdict

CityEvents is resume-ready as a portfolio-grade distributed systems project if the resume wording stays precise.

The strongest story is:

```text
correct transactional core + at-least-once messaging + idempotent business effects + Redis caching + browser auth hardening + CI/E2E evidence + observability + Kubernetes readiness
```

The project is not yet a production HA system.
