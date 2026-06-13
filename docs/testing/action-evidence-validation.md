# GitHub Actions Evidence Validation

## Purpose

This file maps the manual GitHub Actions evidence jobs to the claim they can
support, the failure reason they help diagnose, and the limitation that remains.

Heavy evidence is still GitHub Actions-only. Do not run load, Minikube, or
dependency-failure scripts on the local workstation.

## Evidence Map

| Job / Script | Validates | Failure Usually Means | Does Not Prove |
| --- | --- | --- | --- |
| `kubernetes-live-smoke` / `scripts/k8s-live-smoke.sh` | local Kubernetes manifests apply, pods become ready, gateway workflow works, one pod can be replaced | manifest, image, migration, readiness, service routing, or local overlay bug | production HA, multi-node scheduling, managed dependency HA |
| `load-evidence` / `scripts/load-test-local.sh` | gateway path handles concurrent joins while preserving capacity/waitlist invariants | auth/gateway regression, event-registration locking bug, rate-limit misconfiguration, feed projection lag, or resource saturation | production throughput, autoscaling, cloud load balancer performance |
| `dependency-failure-evidence` / `scripts/failure-test-dependencies.sh` | Redis fallback/fail-open behavior and RabbitMQ outage recovery through outbox | cache fallback bug, revocation fallback bug, relay reconnect bug, broker recovery bug, or projection timeout | production high availability |
| `scripts/verify-phase-4.sh` | scoped Postgres/RabbitMQ integration tests for persistent publish, confirms, retry/DLQ routing, outbox integration, and feed projection idempotency | topology mismatch, broker publish contract bug, RBAC test fixture drift, idempotency regression, or migration mismatch | full-stack Redis/MinIO/Mailpit integration or chaos testing |
| `scripts/inspect-async-ops.sh` | current async state across outbox, retry queues, DLQs, and consumer backlog | tells where the backlog is, not why it began | automatic remediation |

## Load Evidence Reasoning

The load matrix exists to prove correctness under contention, not to produce a
production benchmark.

Key artifact checks:

- `summary.md`: p50/p95/p99/max latency, throughput, success rate, status counts
- `join-results.tsv`: per-request HTTP and domain status
- `dependencies-before-joins.md`: resource and queue state before contention
- `dependencies-after-joins.md`: resource and queue state after contention

How to interpret failures:

- failed join HTTP requests: inspect gateway/auth/event service logs
- `confirmed != min(users, capacity)`: event-registration transaction/locking bug
- `waitlisted != users - confirmed`: waitlist transition bug
- feed projection timeout: outbox relay, RabbitMQ, or feed worker lag/failure
- high queue depth after joins: consumers cannot keep up with producer throughput
- non-zero `DEAD` outbox rows: publish/envelope failures exhausted retry budget

## Dependency Failure Reasoning

The dependency-failure job is intentionally scoped to Redis and RabbitMQ because
those are non-authoritative supporting dependencies in the current design.

Redis outage validates:

- feed reads can fall back to Postgres
- gateway rate limiting follows configured fail-open behavior
- logout still persists token revocation through the durable path

RabbitMQ outage validates:

- event creation can commit source-of-truth state and outbox rows while the broker is down
- outbox rows remain retryable instead of being lost
- relay/workers reconnect after broker recovery
- feed projection eventually catches up

The new async inspection report in `tmp/failure-tests/<run-id>/async-ops-inspection/`
helps explain whether a failure is in outbox state, RabbitMQ queue backlog, retry
queues, DLQs, or consumers.

## Resume Boundary

Allowed after reviewed passing artifacts:

```text
Added GitHub-Actions-only load and dependency-failure evidence for correctness, broker/cache recovery paths, and async backlog diagnosis.
```

Not allowed:

```text
Production high throughput or production high availability.
```
