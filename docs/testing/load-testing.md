# Local Load Testing

## Purpose

`scripts/load-test-local.sh` is a repeatable local HTTP load test for the running CityEvents stack.

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

Start and stop the app automatically:

```bash
./scripts/load-test-local.sh --start-stack --users 80 --capacity 25 --concurrency 20
```

Against an already running local stack:

```bash
./scripts/start-local.sh
./scripts/load-test-local.sh --users 80 --capacity 25 --concurrency 20
```

Results are written to:

```text
tmp/load-test-local/<run-id>/
```

Each run writes:

- `summary.md`
- request/response bodies for setup steps
- per-user join responses
- `join-results.tsv`

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

Use it as local correctness and regression evidence. Strong throughput claims need a controlled environment, repeated runs, resource metrics, database metrics, and failure testing.

## Current Evidence

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
