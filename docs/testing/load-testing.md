# CI Load Evidence

## Purpose

`scripts/load-test-local.sh` is a repeatable HTTP load test for the running
CityEvents stack.

From Phase 15 onward, load evidence is GitHub Actions-only. Do not run this
script from the local workstation. See
`docs/testing/heavy-evidence-runner-policy.md`.

It exercises the public gateway path instead of calling internal services directly:

```text
api-gateway -> auth-service -> event-registration-service -> feed-service
```

## What It Tests

The script verifies:

- seeded admin login
- refresh-cookie CSRF behavior
- organizer registration and admin role promotion
- authenticated event creation through the gateway
- attendee registration
- concurrent event joins through the gateway
- final event capacity invariant
- waitlist count under contention
- eventual feed projection for the created event

The key invariant is:

```text
confirmed joins == min(users, capacity)
waitlisted joins == users - confirmed joins
event detail confirmedCount == confirmed joins
```

## Run

The manual GitHub Actions `Heavy Evidence` workflow starts and stops the app
automatically. It runs a fixed matrix:

| Profile | Users | Capacity | Join concurrency |
| --- | ---: | ---: | ---: |
| small | 40 | 15 | 10 |
| medium | 80 | 25 | 20 |
| stress | 160 | 50 | 40 |

The old local pattern is no longer approved for evidence:

```bash
./scripts/start-local.sh
./scripts/load-test-local.sh --users 80 --capacity 25 --concurrency 20
```

The script now blocks outside GitHub Actions to avoid stressing the workstation.

Results are written to:

```text
tmp/load-test-local/<run-id>/
```

Each run writes:

- `summary.md`
- request/response bodies for setup steps
- per-user join responses
- `join-results.tsv`
- `dependencies-before-joins.md`
- `dependencies-after-joins.md`

The summary includes p50, p95, p99, max latency, join throughput, success
rate, HTTP status counts, domain status counts, and dependency snapshots where
Docker provides them. Dependency snapshots include Postgres active connections,
outbox status counts, RabbitMQ queue depth, and Redis stats.

Failure-case reasoning is recorded in
`docs/testing/action-evidence-validation.md`.

## What It Does Not Prove

This is not a production benchmark.

It does not prove:

- high availability
- autoscaling
- cloud load-balancer behavior
- multi-node Kubernetes scheduling
- managed Postgres/RabbitMQ/Redis behavior
- browser rendering performance
- CDN or TLS termination performance

Use it as CI correctness and regression evidence. Strong throughput claims need repeated runs, resource metrics, database metrics, and failure testing.

The load-evidence acceptance target is defined in
`docs/architecture/phase-15-hardening-rubric.md`: each Actions matrix artifact
should include p50, p95, p99, max latency, throughput, success rate, status
counts, and dependency snapshots where available.

## Accepted Evidence Status

Current accepted evidence is pending the manual GitHub Actions `Heavy Evidence`
workflow matrix artifacts. A resume claim should not say "high throughput" or
"load tested in production" from the historical local run below.

## Historical Local Evidence

This run happened before the Phase 15 workstation-safety policy. Keep it as
development history only; do not repeat it on the local workstation.

Verified on 2026-06-11 with:

```bash
./scripts/load-test-local.sh --start-stack --users 40 --capacity 15 --concurrency 10
```

Observed result:

```text
Join HTTP success: 40/40
Confirmed joins: 15
Waitlisted joins: 25
Event detail confirmedCount: 15
Feed projection: ok
Join latency p95 seconds: 0.156987
Join latency max seconds: 0.183804
```

Run artifacts were written under:

```text
tmp/load-test-local/20260610T234327Z-27605/
```
