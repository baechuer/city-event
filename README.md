# CityEvents V2

CityEvents V2 is a clean rebuild of the city event platform.

The current branch is in Phase 12: final evidence audit and resume framing. It contains the Go service foundation, auth service with short-lived JWT access tokens and rotating HttpOnly refresh tokens, gateway JWT/RBAC boundary, core event-registration consistency boundary, asynchronous messaging path, Redis-backed feed reads, idempotent notification records, asynchronous media metadata processing, a browser demo, basic traceability, Kubernetes-ready manifests, a documented HA deferral, and resume-safe claim guidance.

## Architecture Direction

The intended system is a RabbitMQ-based Go microservices platform:

- `api-gateway`
- `auth-service`
- `event-registration-service`
- `feed-service`
- `notification-service`
- `media-service`
- `media-worker`

The core consistency boundary is `event-registration-service`, which owns event creation, registration, capacity, waitlist, cancellation, promotion, and durable outbox writes.

RabbitMQ is used for asynchronous projections and side effects. Delivery is at least once; consumers must provide idempotent business effects. Redis is used as a non-authoritative cache for feed reads only.

Notification delivery is local-development evidence through Mailpit. Current async messages carry `userId`, not verified email addresses, so production email delivery is still a later claim.

Media uses Postgres metadata plus MinIO object storage locally. Media processing is asynchronous and state-based; it does not sit in the core event-registration transaction.

The frontend is a dependency-free browser app because this workspace has Node but no npm package manager. React, TypeScript, and Vite remain a later frontend-hardening step.

Services emit correlation IDs, structured request logs, and basic Prometheus-style metrics. This is not a full OpenTelemetry/Grafana stack yet.

Kubernetes manifests are provided for deployment readiness with probes, ConfigMaps, Secret templates, ingress routing, and resource limits. They do not prove high availability.

High availability is intentionally documented as a later stage in `docs/architecture/high-availability-decision.md`.

Final evidence and resume wording are documented in `docs/architecture/final-evidence-audit.md` and `docs/resume/resume-claims.md`.

## Local Requirements

- Go 1.25.5
- Docker Desktop with Docker Compose v2
- Bash, such as Git Bash or WSL on Windows

## Run Whole App

The browser demo is the local entry point:

```bash
./scripts/start-local.sh
```

The local stack seeds a development admin account:

```text
email: admin@cityevents.local
password: AdminPass12345
```

Use the admin account to promote test users to `ORGANIZER` before publishing events through the frontend.

Then open:

```text
http://127.0.0.1:18088
```

The launcher starts Docker dependencies, builds and runs all Go services, starts the RabbitMQ workers, and serves the static frontend. The frontend receives runtime API configuration from `/config.js` and defaults to the local API gateway at `http://127.0.0.1:8080`. Press `Ctrl-C` to stop the Go services and frontend.

Auth uses a 15-minute JWT access token kept in browser memory and a rotating opaque refresh token stored as an HttpOnly cookie. The refresh-token hash and token-family state are stored in Postgres.

To point the local frontend at a Kubernetes ingress or another gateway host:

```bash
CITYEVENTS_API_BASE=http://cityevents.local ./scripts/serve-frontend.sh
```

The frontend API methods append `/v1/...`, so the base should be the gateway or ingress origin, not a service-specific path.

From another terminal, stop the local app processes with:

```bash
./scripts/stop-local.sh
```

To also stop Docker Compose dependencies:

```bash
./scripts/stop-local.sh --with-infrastructure
```

To remove local Docker data too, including Postgres, RabbitMQ, Redis, and MinIO volumes:

```bash
./scripts/stop-local.sh --volumes
```

For a non-interactive startup check:

```bash
./scripts/start-local.sh --check
```

## Verify Foundation

```bash
go test ./...
docker compose config --quiet
```

Or run the Phase 1 verification script:

```bash
./scripts/verify-phase-1.sh
```

To also start local infrastructure when Docker Desktop is running:

```bash
./scripts/verify-phase-1.sh --start-infrastructure
```

## Verify Auth Service

Phase 2 auth verification requires Postgres from Docker Compose.

```bash
./scripts/verify-phase-2.sh
```

This runs:

- default Go tests
- auth Postgres integration tests
- runtime smoke for register -> login -> me -> logout -> revoked token fails

## Verify Event Registration Service

Phase 3 event-registration verification requires Postgres from Docker Compose.

```bash
./scripts/verify-phase-3.sh
```

This runs:

- default Go tests
- full integration tests with Postgres
- runtime smoke for create event -> confirmed join -> waitlist join -> cancel and promote -> authoritative status/detail

## Verify RabbitMQ Outbox And Consumers

Phase 4 verification requires Postgres and RabbitMQ from Docker Compose.

```bash
./scripts/verify-phase-4.sh
```

This runs:

- default Go tests
- full integration tests with Postgres and RabbitMQ
- outbox relay stress and retry tests
- feed projection idempotency tests
- worker binary builds for `outbox-relay` and `feed-worker`

## Verify Feed Service

Phase 5 verification requires Postgres, RabbitMQ, and Redis from Docker Compose because the full integration suite includes earlier async tests too.

```bash
./scripts/verify-phase-5.sh
```

This runs:

- default Go tests
- full integration tests with Postgres, RabbitMQ, and Redis
- feed repository and cache fallback tests
- 100-event bounded read test
- feed service build and runtime route smoke

## Verify Notification Service

Phase 6 verification requires Postgres, RabbitMQ, Redis, and Mailpit from Docker Compose because the full integration suite includes earlier phases and local SMTP delivery.

```bash
./scripts/verify-phase-6.sh
```

This runs:

- default Go tests
- full integration tests with Postgres, RabbitMQ, Redis, and Mailpit
- notification idempotency and duplicate-message tests
- provider failure recording tests
- 100-message notification stress test
- local SMTP acceptance test through Mailpit
- notification worker and service builds

## Verify Media Service And Worker

Phase 7 verification requires Postgres, RabbitMQ, Redis, MinIO, and Mailpit from Docker Compose because the full integration suite includes earlier phases too.

```bash
./scripts/verify-phase-7.sh
```

This runs:

- default Go tests
- full integration tests with Postgres, RabbitMQ, Redis, MinIO, and Mailpit
- media metadata repository tests
- MinIO bucket/object tests
- 50-media processing stress test
- concurrent media worker claim test
- media service and worker builds
- media-service runtime smoke for upload intent and detail

## Verify Frontend Product Demo

```bash
./scripts/verify-phase-8.sh
```

This runs:

- backend default Go tests
- frontend JavaScript tests with Node's built-in test runner
- static frontend file checks

To run the browser demo:

```bash
./scripts/start-local.sh
```

Then open:

```text
http://127.0.0.1:18088
```

## Verify Observability And Debugging

Phase 9 verification requires the full local dependency stack because it reruns the integration suite and checks the debugging walkthrough.

```bash
./scripts/verify-phase-9.sh
```

This runs:

- default Go tests
- full integration tests
- correlation ID and metrics tests
- outbox correlation propagation test
- debugging walkthrough file checks

## Verify Kubernetes Readiness

Phase 10 verification validates the local test suite, Docker Compose config, Kubernetes manifest coverage, probes, resource limits, and optional `kubectl` dry-run when `kubectl` is available.

```bash
./scripts/verify-phase-10.sh
```

## Verify High Availability Decision

Phase 11 verification keeps the project honest: it verifies the Kubernetes readiness evidence still passes and confirms the repository does not overclaim high availability.

```bash
./scripts/verify-phase-11.sh
```

## Verify Final Evidence Audit

Phase 12 verification reruns the Kubernetes/HA claim gates and checks final resume evidence documents.

```bash
./scripts/verify-phase-12.sh
```

For the strongest local evidence run, include full Docker-backed integration tests:

```bash
./scripts/verify-phase-12.sh --run-full-integration
```

## Run Local Infrastructure

```bash
docker compose up -d
docker compose ps
```

Local dependency ports:

| Dependency | URL |
| --- | --- |
| Postgres | `localhost:5432` |
| RabbitMQ AMQP | `localhost:5672` |
| RabbitMQ UI | `http://localhost:15672` |
| Redis | `localhost:6379` |
| MinIO API | `http://localhost:9000` |
| MinIO Console | `http://localhost:9001` |
| Mailpit UI | `http://localhost:8025` |

Default local credentials are for development only:

- Postgres: `cityevents` / `cityevents`
- RabbitMQ: `cityevents` / `cityevents`
- MinIO: `cityevents` / `cityevents-password`

## Run Service Skeletons

Each service exposes `/livez` and `/readyz`.

```bash
go run ./cmd/api-gateway
go run ./cmd/auth-service
go run ./cmd/event-registration-service
go run ./cmd/feed-service
go run ./cmd/notification-service
go run ./cmd/media-service
go run ./cmd/outbox-relay
go run ./cmd/feed-worker
go run ./cmd/notification-worker
go run ./cmd/media-worker
```

Default service ports:

| Service | Health URL |
| --- | --- |
| api-gateway | `http://localhost:8080/readyz` |
| auth-service | `http://localhost:8081/readyz` |
| event-registration-service | `http://localhost:8082/readyz` |
| feed-service | `http://localhost:8083/readyz` |
| notification-service | `http://localhost:8084/readyz` |
| media-service | `http://localhost:8085/readyz` |
| media-worker | `http://localhost:8086/readyz` |

For startup wiring checks that exit immediately:

```bash
export CITYEVENTS_STARTUP_CHECK_ONLY=true
go run ./cmd/api-gateway
go run ./cmd/auth-service
go run ./cmd/event-registration-service
go run ./cmd/feed-service
go run ./cmd/notification-service
go run ./cmd/media-service
go run ./cmd/media-worker
unset CITYEVENTS_STARTUP_CHECK_ONLY
```

## Phase 12 Claim Boundary

Allowed claim:

```text
Built a portfolio-grade Go microservices event platform with gateway JWT/RBAC, rotating refresh tokens, PostgreSQL-backed event registration, RabbitMQ asynchronous workflows, Redis feed caching, idempotent consumers, observability hooks, Kubernetes-ready manifests, and documented production/HA limitations.
```

Not yet allowed:

```text
Exactly-once RabbitMQ consumption, guaranteed no message loss, highly available Kubernetes deployment, autoscaling under load, production cluster deployment, HA RabbitMQ/Postgres/Redis, or failure-tested recovery.
```
