# CityEvents V2

CityEvents V2 is a clean rebuild of the city event platform.

The current branch is in Phase 4: RabbitMQ outbox and consumers. It contains the Go service foundation, auth service, core event-registration consistency boundary, and asynchronous messaging path.

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

RabbitMQ is used for asynchronous projections and side effects. Delivery is at least once; consumers must provide idempotent business effects.

## Local Requirements

- Go 1.25.5
- Docker Desktop with Docker Compose v2

## Verify Foundation

```powershell
go test ./...
docker compose config --quiet
```

Or run the Phase 1 verification script:

```powershell
.\scripts\verify-phase-1.ps1
```

To also start local infrastructure when Docker Desktop is running:

```powershell
.\scripts\verify-phase-1.ps1 -StartInfrastructure
```

## Verify Auth Service

Phase 2 auth verification requires Postgres from Docker Compose.

```powershell
.\scripts\verify-phase-2.ps1
```

This runs:

- default Go tests
- auth Postgres integration tests
- runtime smoke for register -> login -> me -> logout -> revoked token fails

## Verify Event Registration Service

Phase 3 event-registration verification requires Postgres from Docker Compose.

```powershell
.\scripts\verify-phase-3.ps1
```

This runs:

- default Go tests
- full integration tests with Postgres
- runtime smoke for create event -> confirmed join -> waitlist join -> cancel and promote -> authoritative status/detail

## Verify RabbitMQ Outbox And Consumers

Phase 4 verification requires Postgres and RabbitMQ from Docker Compose.

```powershell
.\scripts\verify-phase-4.ps1
```

This runs:

- default Go tests
- full integration tests with Postgres and RabbitMQ
- outbox relay stress and retry tests
- feed projection idempotency tests
- worker binary builds for `outbox-relay` and `feed-worker`

## Run Local Infrastructure

```powershell
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

```powershell
go run ./cmd/api-gateway
go run ./cmd/auth-service
go run ./cmd/event-registration-service
go run ./cmd/feed-service
go run ./cmd/notification-service
go run ./cmd/media-service
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

```powershell
$env:CITYEVENTS_STARTUP_CHECK_ONLY='true'
go run ./cmd/api-gateway
go run ./cmd/auth-service
go run ./cmd/event-registration-service
go run ./cmd/feed-service
go run ./cmd/notification-service
go run ./cmd/media-service
go run ./cmd/media-worker
Remove-Item Env:\CITYEVENTS_STARTUP_CHECK_ONLY
```

## Phase 4 Claim Boundary

Allowed claim:

```text
Implemented RabbitMQ-based asynchronous decoupling using a transactional outbox, publisher confirms, persistent messages, and idempotent consumers for eventual-consistency projections.
```

Not yet allowed:

```text
Exactly-once RabbitMQ consumption, full feed ranking/search, email notification delivery, or high availability.
```
