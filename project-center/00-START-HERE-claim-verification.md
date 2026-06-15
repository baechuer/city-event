# CityEvents Claim Verification And Architecture Brief

Last verified: 2026-06-15, local Windows workstation. React/TypeScript frontend migration, local stack health, and seeded-admin login verified.

This is the first document to read before making resume claims about CityEvents. It describes what the project is, what it can and cannot currently prove, why the architecture was chosen, what tradeoffs were accepted, where the relevant code lives, and what evidence has been run.

## Executive Verdict

CityEvents is a local event discovery and registration platform rebuilt as a Go microservices portfolio project with a React + TypeScript frontend. Users can create accounts, browse events, join events, publish events as organizers, moderate registrations as organizers/admins, receive projected feed updates and notification records, and create media upload intents backed by object storage.

The project currently supports a resume claim of:

> Built a Go microservices event platform with a React + TypeScript frontend, an API gateway, JWT/RBAC security, PostgreSQL transactional consistency for event registration, RabbitMQ asynchronous projections, Redis-backed caching/rate limiting/token-revocation acceleration, local object/email infrastructure, CI/security gates, Kubernetes manifests, and explicit at-least-once plus idempotent-consumer reliability boundaries.

The project should not currently claim:

- "Exactly-once delivery" or "exactly-once consumption" across RabbitMQ and services.
- "Messages will always be stored" under every possible crash or infrastructure failure.
- "Production high availability has been proven."
- "Production deployed on Kubernetes."
- "Real payment-grade no-side-effect guarantees."

The accurate claim is stronger and safer:

> Critical event-registration state is committed transactionally in PostgreSQL. Domain events are persisted in the same database transaction through the outbox pattern, then asynchronously published to RabbitMQ with retry and terminal dead-letter behavior. Consumers are idempotent at the business/projection layer, so duplicate deliveries should not create duplicate business effects.

## Product Scope

CityEvents targets a city/community event workflow:

- Public visitors browse event feed data.
- Users register/login and maintain an authenticated session.
- Organizers publish, update, and cancel events.
- Users join events until capacity is full.
- Overflow users are waitlisted.
- Canceling a confirmed registration promotes the next waitlisted user.
- Organizers/admins can cancel someone else's registration.
- Admins can update user roles.
- Feed and notification state are eventually consistent projections.
- Media upload intent is created through the backend and stored through MinIO-compatible object storage.

Current non-goals:

- No real payment provider integration.
- No real email/SMS provider integration; Mailpit is local evidence only.
- No production object bucket; MinIO is local S3-compatible evidence.
- No live production/staging cluster, managed database, managed broker, or managed cache.
- No production CDN/static hosting path, visual regression suite, or formal accessibility audit for the frontend.
- No real OpenTelemetry SDK exporter spans yet; correlation IDs, W3C trace context, metrics, dashboard/alert config, and collector config exist, but full distributed trace visualization remains future work.
- No reviewed heavy-evidence artifacts yet for load, dependency-failure, or live Kubernetes failure testing.

## High-Level Architecture

```text
Browser frontend (React + TypeScript + Vite)
  -> runtime /config.js selects API/gateway base URL
  -> API Gateway
      -> Auth Service
      -> Event Registration Service
      -> Feed Service
      -> Media Service

Event Registration Service
  -> PostgreSQL transaction
      -> events / registrations tables
      -> outbox_messages table

Outbox Relay
  -> reads outbox_messages with SKIP LOCKED
  -> publishes persistent RabbitMQ messages with confirms
  -> marks outbox row SENT / FAILED / DEAD

RabbitMQ
  -> feed projection worker
  -> notification worker
  -> retry exchanges and DLQs

Feed Service
  -> PostgreSQL feed_events projection
  -> Redis cache for read optimization

Notification Service / Worker
  -> PostgreSQL notification ledger
  -> local SMTP/Mailpit provider

Media Service / Worker
  -> PostgreSQL media metadata
  -> MinIO object storage
```

## Service Functionality And Traffic Flow

This section maps the actual services to what they do, how traffic reaches them, whether the API is public or internal, the code that implements it, and the design alternatives behind each choice.

### Public vs Private Boundary

In the intended architecture, the browser talks to the API gateway. The gateway is the public HTTP boundary. The individual backend services also expose HTTP servers for local/dev simplicity, but conceptually they are internal services behind the gateway and Kubernetes service network.

Public through gateway:

- `GET /v1/events`: public event list.
- `GET /v1/events/{eventID}`: public detail, optionally personalized if a bearer token is present.
- `GET /v1/feed/events`: public projected feed.
- `GET /v1/feed/events/{eventID}`: public projected feed detail.
- `GET /v1/media/{mediaID}`: public media metadata read.
- `/v1/auth/register`, `/v1/auth/login`, `/v1/auth/refresh`: public auth/session entry points.

Authenticated through gateway:

- `POST /v1/events`: organizer/admin creates event.
- `PATCH /v1/events/{eventID}`: organizer/admin updates event.
- `POST /v1/events/{eventID}/cancel`: organizer/admin cancels event.
- `POST /v1/events/{eventID}/join`: user joins event.
- `DELETE /v1/events/{eventID}/join`: user cancels own registration.
- `DELETE /v1/events/{eventID}/registrations/{userID}`: organizer/admin cancels another user's registration.
- `GET /v1/events/{eventID}/join`: authenticated user's join status.
- `POST /v1/media/uploads`: authenticated upload intent.
- `POST /v1/media/{mediaID}/uploaded`: authenticated upload completion.
- `GET /v1/auth/me`, `/v1/auth/logout`, `/v1/auth/users/{userID}/role`: authenticated auth routes, with role checks for role update.

Relevant code:

- Gateway route wiring: `internal/services/gateway/http.go`
- Gateway auth mode decision: `eventAuthMode`, `proxyEvents`, `proxyMedia`, `authenticate`, `requestWithPrincipal` in `internal/services/gateway/http.go`
- Shared health/CORS/metrics/rate-limit wrapper: `internal/platform/httpapi/router.go`

Design option:

- Expose every service directly to the frontend.

Why not:

- More public attack surface.
- More duplicated CORS/auth behavior.
- Frontend must know internal topology.

Chosen design:

- Gateway as public boundary, internal services as private implementation detail.

Tradeoff:

- Gateway adds another hop and must be highly available.
- Gateway failures can affect the whole browser API.

### Traffic Flow: Browse Feed

```text
Browser
  -> API Gateway GET /v1/feed/events
  -> Feed Service
  -> Redis cache lookup
  -> PostgreSQL feed_events fallback/query
  -> API Gateway
  -> Browser
```

What it does:

- Returns query-optimized event feed data.
- Supports filters such as city, limit, and offset.
- Uses Redis as a cache, with Postgres as source-of-truth for projected feed data.

Relevant code:

- Route: `internal/services/feed/http.go`
- Service/cache: `internal/services/feed/service.go`, `internal/services/feed/cache.go`
- Repository: `internal/services/feed/repository.go`

Design option:

- Query the event-registration service directly for every feed request.

Why not:

- Feed reads can become high volume.
- Feed shape may differ from write model.
- Keeping feed as projection protects the write service from read traffic.

Tradeoff:

- Feed can be stale until workers process RabbitMQ messages.

### Traffic Flow: Register/Login/Refresh

```text
Browser
  -> API Gateway /v1/auth/*
  -> Auth Service
  -> PostgreSQL users / revoked tokens / refresh sessions
  -> optional Redis revocation cache
  -> Browser receives access token + refresh cookie
```

What it does:

- Registers users.
- Logs users in.
- Issues short-lived access tokens.
- Stores refresh token in HttpOnly cookie.
- Rotates refresh tokens on refresh.
- Revokes access token IDs and refresh sessions on logout.
- Allows admins to update roles.

Relevant endpoints:

- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `POST /v1/auth/refresh`
- `GET /v1/auth/me`
- `POST /v1/auth/logout`
- `PATCH /v1/auth/users/{userID}/role`

Relevant code:

- Routes and cookies/CSRF: `internal/services/auth/http.go`
- Business logic: `internal/services/auth/service.go`
- JWT implementation: `internal/services/auth/token.go`
- Persistence: `internal/services/auth/postgres_repository.go`
- Revocation cache: `internal/services/auth/revocation_cache.go`
- Frontend memory-token behavior: `frontend/src/api.ts`

Design option:

- Store long-lived JWT in localStorage.

Why not:

- Higher persistence risk if XSS occurs.

Chosen design:

- Short-lived in-memory access token plus rotating HttpOnly refresh token.

Tradeoff:

- Refresh flow and CSRF protection are more complex.
- Access token theft is still possible during the token TTL.

### Traffic Flow: Authenticated Event Mutation

```text
Browser
  -> API Gateway with Authorization: Bearer token
  -> Gateway calls Auth Service /v1/auth/me
  -> Gateway injects X-User-ID and X-User-Role
  -> Event Registration Service
  -> PostgreSQL transaction
  -> outbox_messages row
  -> response to Browser
```

What it does:

- Creates, updates, and cancels events.
- Requires authenticated organizer/admin role for event publishing and mutation.
- Writes outbox rows in the same transaction as event changes.

Relevant endpoints:

- `POST /v1/events`
- `PATCH /v1/events/{eventID}`
- `POST /v1/events/{eventID}/cancel`

Relevant code:

- Gateway principal injection: `requestWithPrincipal` in `internal/services/gateway/http.go`
- Event routes: `internal/services/eventregistration/http.go`
- Role validation/business logic: `internal/services/eventregistration/service.go`
- Transaction/outbox insert: `CreateEvent`, `CancelEvent`, `insertOutbox` in `internal/services/eventregistration/postgres_repository.go`

Design option:

- Let frontend send `X-User-ID` directly to event service.

Why not:

- Anyone could spoof another user/role.

Chosen design:

- Gateway strips incoming identity headers, verifies JWT with auth service, then injects trusted identity headers.

Tradeoff:

- Event service trusts gateway/header boundary, so network policy and private service exposure matter in production.

### Traffic Flow: Join Event And Waitlist

```text
Browser
  -> API Gateway POST /v1/events/{id}/join
  -> Auth Service validation via gateway
  -> Event Registration Service
  -> PostgreSQL transaction:
       lock event row
       check active registration
       count confirmed registrations
       decide CONFIRMED or WAITLISTED
       insert registration
       insert outbox row
  -> response with join status
```

What it does:

- Confirms users while capacity remains.
- Waitlists users after capacity fills.
- Handles duplicate join attempts as existing active registrations.
- Supports `Idempotency-Key` for safer repeated client requests.

Relevant code:

- Route: `joinEvent` in `internal/services/eventregistration/http.go`
- Business entry: `JoinEvent` in `internal/services/eventregistration/service.go`
- Transaction logic: `JoinEvent`, `selectEventForUpdate`, `confirmedCountTx`, `activeRegistrationTx`, `nextWaitlistPositionTx`, `insertOutbox` in `internal/services/eventregistration/postgres_repository.go`
- Domain decision: `DecideJoinStatus` in `internal/services/eventregistration/domain.go`
- Tests: `internal/services/eventregistration/service_test.go`, `internal/services/eventregistration/postgres_repository_integration_test.go`

Design option:

- Use a queue for all joins and process them asynchronously.

Why not:

- User would not immediately know confirmed vs waitlisted.
- UX becomes harder.
- The system still needs a consistent source of truth eventually.

Chosen design:

- Synchronous transaction for join decision; asynchronous fanout after commit.

Tradeoff:

- Stronger correctness but lower throughput on very hot events due to row locking.

### Traffic Flow: Cancel Registration And Promote Waitlist

```text
Browser
  -> API Gateway DELETE /v1/events/{id}/join
     or DELETE /v1/events/{id}/registrations/{userID}
  -> Event Registration Service
  -> PostgreSQL transaction:
       lock event
       cancel registration
       if confirmed slot opened, promote first waitlisted
       insert cancellation and promotion outbox rows
  -> response with canceled/promoted state
```

What it does:

- Lets a user cancel their own registration.
- Lets organizer/admin cancel another user's registration.
- Promotes earliest waitlisted user when a confirmed slot opens.

Relevant code:

- Routes: `cancelJoin`, `cancelRegistration` in `internal/services/eventregistration/http.go`
- Business entry: `CancelJoin`, `CancelRegistration` in `internal/services/eventregistration/service.go`
- Transaction logic: `CancelJoin`, `CancelRegistration`, `cancelRegistrationTx`, `firstWaitlistedTx`, `insertOutbox` in `internal/services/eventregistration/postgres_repository.go`

Design option:

- Promote waitlist asynchronously in a worker.

Why not:

- Capacity could temporarily be inconsistent.
- User cancellation response would not know who was promoted.
- More edge cases if multiple cancels happen quickly.

Chosen design:

- Promotion is part of the same transaction as cancellation.

Tradeoff:

- More work inside the transaction, but stronger invariant.

### Traffic Flow: Outbox Relay To RabbitMQ

```text
Outbox Relay process
  -> PostgreSQL outbox_messages
  -> SELECT available rows FOR UPDATE SKIP LOCKED
  -> RabbitMQ publish with confirms
  -> mark row SENT, FAILED, or DEAD
```

What it does:

- Moves committed domain events from Postgres to RabbitMQ.
- Allows multiple relay instances to work without selecting the same row.
- Applies retry backoff and terminal dead state.

Relevant code:

- Worker entry: `cmd/outbox-relay/main.go`
- Relay logic: `PublishBatch`, `selectAvailable`, `markSent`, `markFailed`, `markDead`, `retryBackoff` in `internal/services/outboxrelay/relay.go`
- RabbitMQ publisher: `internal/platform/messaging/rabbitmq.go`

Design option:

- Publish inside the event-registration HTTP transaction.

Why not:

- External broker call inside DB transaction increases latency and creates awkward partial-failure behavior.

Design option:

- Use CDC instead of polling outbox table.

Why not yet:

- More production-grade at scale, but heavier infrastructure for this project.

Chosen design:

- Polling transactional outbox with `SKIP LOCKED`.

Tradeoff:

- Polling adds some latency and needs cleanup/retention work later.

### Traffic Flow: RabbitMQ To Feed Projection

```text
RabbitMQ feed queue
  -> Feed Worker
  -> decode envelope
  -> processed_messages idempotency claim
  -> update feed_events projection
  -> ack message
```

What it does:

- Converts domain events into read-optimized feed rows.
- Handles event published/updated/canceled and join count transitions.
- Deduplicates by processed message ID.

Relevant code:

- Worker entry: `cmd/feed-worker/main.go`
- Consumer: `internal/services/feedprojection/consumer.go`
- Projector: `HandleEnvelope`, `claimMessage`, `apply`, `upsertEvent`, `changeConfirmedCount` in `internal/services/feedprojection/projector.go`
- Messaging topology/retry: `internal/platform/messaging/rabbitmq.go`

Design option:

- Feed service directly queries registration tables.

Why not:

- Read model would be coupled to write schema and heavier joins.

Chosen design:

- Separate projection table optimized for feed reads.

Tradeoff:

- Eventual consistency and projection repair/backfill requirements.

### Traffic Flow: RabbitMQ To Notification Delivery

```text
RabbitMQ notification queue
  -> Notification Worker
  -> decode envelope
  -> notification_processed_messages idempotency claim
  -> insert notification intent as PENDING
  -> ACK RabbitMQ message
  -> delivery loop claims due notification with SKIP LOCKED
  -> send through SMTP provider with deterministic idempotency metadata
  -> insert delivery record
  -> mark SENT or FAILED with retry state
```

What it does:

- Decides whether a domain event should create a notification.
- Stores notification intent and delivery state.
- Sends local email through Mailpit in development/testing.
- Deduplicates processed message IDs.
- Separates RabbitMQ message handling from provider delivery so provider failure does not require message redelivery once the intent is durable.

Relevant code:

- Worker entry: `cmd/notification-worker/main.go`
- Consumer: `internal/services/notification/consumer.go`
- Decision/domain: `internal/services/notification/domain.go`
- Intent processing and persistence: `ProcessEnvelope`, `claimMessage`, `insertNotification` in `internal/services/notification/repository.go`
- Delivery worker and retry state: `DeliveryWorker`, `ClaimNextDelivery`, `MarkDeliverySent`, `MarkDeliveryFailed`, `deliveryBackoff` in `internal/services/notification/repository.go`
- Provider boundary: `SMTPProvider.Send`, `smtpMessageID` in `internal/services/notification/provider.go`

Design option:

- Send notification directly inside join/cancel transaction.

Why not:

- Email/provider failure should not roll back a successful event registration.
- Provider latency should not slow down the user command path.

Chosen design:

- Async notification worker with durable intent creation and a separate retryable provider delivery loop.

Tradeoff:

- Notifications can be delayed or fail after bounded retries; user-facing event-registration state remains correct.
- SMTP `Message-ID` is not the same as real provider idempotency. A production provider should use provider-supported idempotency keys where available.

### Traffic Flow: Media Upload

```text
Browser
  -> API Gateway POST /v1/media/uploads
  -> Media Service creates DB asset + presigned URL
  -> Browser uploads object to MinIO URL
  -> Browser marks uploaded
  -> Media Worker claims uploaded asset
  -> checks MinIO object exists
  -> marks READY or FAILED
```

What it does:

- Creates media metadata.
- Issues presigned upload URLs.
- Tracks upload lifecycle: uploading, uploaded, processing, ready, failed.
- Verifies object existence asynchronously.

Relevant code:

- Routes: `internal/services/media/http.go`
- Service: `CreateUpload`, `MarkUploaded`, `Get`, `Worker.ProcessOne` in `internal/services/media/service.go`
- Repository/claiming: `Create`, `MarkUploaded`, `ClaimNextUploaded` in `internal/services/media/repository.go`
- Storage: `internal/services/media/storage.go`
- Worker entry: `cmd/media-worker/main.go`

Design option:

- Upload file bytes through the API service.

Why not:

- API service becomes a bandwidth bottleneck.
- Large file handling complicates service scaling.

Chosen design:

- Presigned URL upload to object storage.

Tradeoff:

- More states to manage.
- Client must complete upload and mark completion.
- Worker must verify object existence and handle missing object failures.

### Traffic Flow: Health, Metrics, Rate Limit, And Tracing

```text
Any HTTP request
  -> service base router
  -> CORS middleware
  -> observability middleware
  -> rate-limit middleware
  -> service handler
```

What it does:

- Adds `/livez`, `/readyz`, and `/metrics` to services.
- Propagates or creates `X-Correlation-ID`.
- Propagates or creates W3C `traceparent`.
- Applies rate limits.
- Records HTTP and rate-limit metrics.

Relevant code:

- Base router: `internal/platform/httpapi/router.go`
- Observability middleware: `internal/platform/httpapi/observability.go`
- Metrics registry: `internal/platform/observability/observability.go`
- Rate limiting: `internal/platform/httpapi/rate_limit.go`
- Config: `internal/platform/config/config.go`

Design option:

- Implement CORS/logging/rate limit separately in each service.

Why not:

- Inconsistent behavior.
- More duplicated code.
- Higher chance one service misses a security/observability control.

Chosen design:

- Shared platform router/middleware.

Tradeoff:

- Shared platform code becomes a dependency across services; changes need careful regression tests.

### End-To-End Example: User Joins Event

```text
Browser POST /v1/events/{id}/join
  -> API Gateway validates JWT by calling Auth Service /v1/auth/me
  -> Gateway removes untrusted identity headers and injects trusted X-User-ID/X-User-Role
  -> Event Registration Service locks event row and decides CONFIRMED/WAITLISTED
  -> Event Registration Service writes registration + outbox row in one transaction
  -> Outbox Relay publishes join.confirmed or join.waitlisted to RabbitMQ
  -> Feed Worker updates confirmed count projection when relevant
  -> Notification Worker records notification intent when relevant
  -> Notification delivery loop sends local SMTP message later
  -> Feed Service eventually returns updated projected count
```

What this shows in an interview:

- Authentication and identity propagation.
- Strong consistency in the command path.
- Transactional outbox for reliable async fanout.
- At-least-once messaging with idempotent projections.
- Eventual consistency for read/fanout paths.

### End-To-End Example: Organizer Cancels Someone Else's Registration

```text
Browser DELETE /v1/events/{id}/registrations/{userID}
  -> API Gateway authenticates organizer/admin
  -> Event Registration Service checks actor is event organizer or admin
  -> Transaction cancels target registration
  -> If target was CONFIRMED, first WAITLISTED user is promoted
  -> Outbox rows capture cancellation and promotion
  -> Feed and notification workers process async updates
```

What this shows in an interview:

- RBAC is not just "logged in"; action ownership matters.
- Admin and organizer powers are different from normal user powers.
- Promotion is synchronous because it affects capacity correctness.

### Service Responsibility Summary

| Component | Public or private | Main responsibility | Main storage/dependency | Key code |
| --- | --- | --- | --- | --- |
| Frontend | Public browser UI | React/TypeScript product demo, auth/session client, event/feed/media workflows | Browser memory/cookies | `frontend/src/App.tsx`, `frontend/src/api.ts`, `frontend/src/state.ts` |
| API Gateway | Public API boundary | Route fanout, JWT validation, trusted identity headers | Auth service | `internal/services/gateway/http.go` |
| Auth Service | Internal behind gateway, public auth routes proxied | Users, login, JWT, refresh rotation, role updates | PostgreSQL, Redis revocation cache | `internal/services/auth/*` |
| Event Registration Service | Internal behind gateway | Events, joins, waitlist, moderation, outbox writes | PostgreSQL | `internal/services/eventregistration/*` |
| Outbox Relay | Private worker | Publish committed outbox rows to RabbitMQ | PostgreSQL, RabbitMQ | `cmd/outbox-relay/main.go`, `internal/services/outboxrelay/relay.go` |
| RabbitMQ | Private infrastructure | Durable async delivery, retry, DLQ | RabbitMQ queues/exchanges | `internal/platform/messaging/rabbitmq.go` |
| Feed Worker | Private worker | Build feed projection from messages | RabbitMQ, PostgreSQL | `cmd/feed-worker/main.go`, `internal/services/feedprojection/*` |
| Feed Service | Internal behind gateway, public feed routes proxied | Serve projected feed reads | PostgreSQL, Redis | `internal/services/feed/*` |
| Notification Worker | Private worker | Create/dedupe notification intents and run provider delivery loop | RabbitMQ, PostgreSQL, SMTP | `cmd/notification-worker/main.go`, `internal/services/notification/*` |
| Notification Service | Internal service shell | Health/build surface for notification domain | PostgreSQL | `cmd/notification-service/main.go` |
| Media Service | Internal behind gateway | Upload intents, asset metadata, upload completion | PostgreSQL, MinIO | `internal/services/media/*` |
| Media Worker | Private worker | Verify uploaded objects and mark ready/failed | PostgreSQL, MinIO | `cmd/media-worker/main.go`, `internal/services/media/service.go` |

## Main Design Choices

### Microservices With A Small API Gateway

Chosen because the resume goal is to demonstrate service boundaries, independent deployable processes, async messaging, and operational tradeoffs. The gateway gives the browser one public API entry point, centralizes JWT verification/proxy behavior, and keeps internal services simpler.

Alternative: monolith.

Tradeoff: a monolith would be easier to debug and deploy for this product size. Microservices add local orchestration, network boundaries, migrations per service area, tracing needs, and more failure modes. The project accepts that overhead because the learning/resume objective is distributed-system design, not fastest product delivery.

### Event And Registration Are Fused

Chosen because event capacity, registration, waitlist promotion, and organizer/admin cancellation are one consistency boundary. Splitting event service and join service would make every join/cancel path a cross-service distributed transaction or saga.

Alternative: separate event catalog service and registration service.

Tradeoff: fusing these keeps correctness simpler but makes the event-registration service larger. This is justified because capacity and waitlist invariants are high-value synchronous rules.

### Go With Lightweight HTTP Frameworks

Chosen because the backend is a set of small network services where explicit control over handlers, middleware, transactions, and process boundaries matters more than framework speed. Go gives simple static-ish binaries, low runtime overhead, strong concurrency primitives, explicit error handling, and mature libraries for HTTP, PostgreSQL, Redis, RabbitMQ, object storage, and testing.

Alternatives:

- Node/TypeScript backend: faster full-stack iteration and shared language with the frontend, but weaker fit for demonstrating Go service engineering and lower-level operational control.
- Python/FastAPI: very fast API development, but less compelling for concurrent worker/service binaries in this resume story.
- Java/Spring: enterprise-ready and feature-rich, but heavier local footprint and more framework-driven for this project scale.
- Go framework such as Gin/Fiber: productive routing, but less need here because Chi plus standard library keeps middleware and behavior visible.

Tradeoff: Go is more verbose than Node/Python for CRUD and the lightweight approach means more wiring is hand-authored. That is acceptable here because the project is meant to demonstrate architecture, transactions, security middleware, and reliability boundaries clearly.

### React + TypeScript + Vite Frontend

Chosen because the frontend now has real product workflows: browse feed, inspect events, register/login, refresh sessions, publish as organizer/admin, join/cancel events, moderate attendees, update roles as admin, and create media upload intents. React gives a component model for stateful UI, TypeScript documents the API contract at compile time, and Vite gives fast local iteration plus a production bundle served by `frontend/server.mjs`.

Alternatives:

- Static vanilla JavaScript: simpler and dependency-light, but becomes harder to maintain as auth/session state, role-gated UI, forms, and route-like views grow.
- Next.js/Remix: stronger routing/server-rendering story, but unnecessary because the backend API gateway already owns server behavior and the frontend is a local SPA/product demo.
- Full component library: faster polish, but can hide design decisions and increase bundle/dependency surface.

Tradeoff: React adds build tooling, dependencies, and client-side state complexity. The project keeps the frontend bounded as a workflow demo and uses runtime `/config.js` so gateway/ingress targets can change without rebuilding the bundle.

### PostgreSQL As Source Of Truth

Chosen for relational constraints, transactional joins/cancellations, row locks, and auditability.

Alternatives: document DB, event store, Redis-first model.

Tradeoff: PostgreSQL is not infinite throughput by itself. Scaling hot events needs careful locking, sharding, queue admission, or partitioning later. For entry-level resume evidence, PostgreSQL gives the clearest correctness story.

### RabbitMQ For Async Work

Chosen because RabbitMQ is simpler than Kafka/Pulsar for command/event delivery in a small service platform, supports durable queues, publisher confirms, retry routing, and DLQs.

Alternatives:

- Kafka: better for high-throughput event streams and replay, heavier operational model.
- Pulsar: strong messaging features and scaling, higher complexity.
- AWS SQS FIFO: managed deduplication/order within limits, but cloud-specific and still not "exactly-once business effects" without idempotent handlers.

Tradeoff: RabbitMQ gives at-least-once delivery. Consumers must handle duplicates. That is acceptable here because feed and notification consumers persist processed message IDs.

### Redis As Optimization, Not Source Of Truth

Chosen for fast cache and coordination-adjacent operations that can tolerate fallback behavior: feed response caching, Redis-backed rate limiting, and accelerated token-revocation checks.

Alternatives:

- PostgreSQL only: simpler and more durable, but puts all read/rate-limit/revocation lookup pressure on the source-of-truth database.
- In-memory only: easiest locally, but every service replica would enforce different counters and caches.
- Dedicated API gateway/WAF rate limiting: stronger production edge control, but outside this local portfolio scope.

Tradeoff: Redis adds another dependency and can fail independently. CityEvents treats Redis as an accelerator: feed reads can fall back to PostgreSQL, revocation checks fall back to durable Postgres state, and rate limiting has an explicit fail-open/fail-closed choice.

### Transactional Outbox

Chosen because direct "write DB then publish RabbitMQ" can lose messages if the service crashes after DB commit but before publish. The outbox writes the event record and outbox row in the same PostgreSQL transaction.

Alternatives:

- Direct publish after DB commit: simpler but can lose publish events.
- Publish before DB commit: can publish events for rolled-back data.
- CDC from database log: robust and production-friendly, but adds Debezium/Kafka-like infrastructure.
- Event sourcing: powerful but changes the whole domain model.
- Two-phase commit: complex and rarely worth it for RabbitMQ plus Postgres.

Tradeoff: outbox adds relay logic, outbox cleanup needs, and eventual consistency latency.

### Retry, Backoff, DLQ, And Replay Guardrails

Chosen because endless retry hides failures and can overload dependencies. Producer-side publish failures move through outbox `FAILED` backoff states and eventually `DEAD`. Consumer-side poison messages are republished into delayed retry queues with increasing delay and then copied to DLQ after retry exhaustion.

Alternatives:

- Infinite retry: simple but dangerous during persistent bugs or bad payloads.
- Drop failed messages: protects throughput but loses evidence and makes repair impossible.
- Managed broker-native redrive: useful in cloud systems, but this project keeps the mechanism visible through RabbitMQ topology and scripts.

Tradeoff: DLQs do not fix the bug. They preserve failed work for inspection. Replay tools must be guarded because replaying bad messages can re-trigger the same failure or duplicate external effects.

### Effectively-Once Business Effects, Not Exactly-Once Delivery

RabbitMQ and most queues support at-least-once because acknowledging a message and committing consumer side effects cannot be made atomic across two independent systems without extra coordination. A crash between "side effect committed" and "ack sent" can cause redelivery.

CityEvents handles this with idempotency:

- Outbox message ID is stable and used as message identity.
- Feed projection stores processed messages in `processed_messages`.
- Notification intent creation stores processed messages in `notification_processed_messages`; provider delivery then uses local delivery leases plus provider idempotency metadata.
- Event join has idempotency key support and database uniqueness/concurrency tests.
- Outbox terminal failure becomes `DEAD`, and consumer poison messages route to DLQ after retry.

This is the correct resume wording:

> At-least-once messaging with idempotent consumers and effectively-once business effects for feed projections and notification-intent records. External email delivery is best-effort unless the provider enforces idempotency.

## Key Workflows

### Register/Login/Session

1. User registers or logs in through auth routes.
2. Auth service returns a short-lived JWT access token.
3. Refresh token is stored as an HttpOnly cookie.
4. Refresh token rotation creates a new refresh token and invalidates/replaces the old one.
5. Reuse of an old refresh token revokes the refresh family.
6. Logout revokes access token ID and refresh session.

Relevant code:

- `internal/services/auth/service.go`
- `internal/services/auth/http.go`
- `internal/services/auth/token.go`
- `internal/services/auth/postgres_repository.go`
- `internal/services/auth/revocation_cache.go`
- `frontend/src/api.ts`

### Publish Event

1. Organizer/admin sends create event request.
2. Event-registration service validates role and input.
3. PostgreSQL transaction inserts event.
4. Same transaction inserts `outbox_messages` row.
5. Outbox relay later publishes RabbitMQ message.
6. Feed/notification workers eventually project the event.

Relevant code:

- `internal/services/eventregistration/service.go`
- `internal/services/eventregistration/postgres_repository.go`
- `internal/services/outboxrelay/relay.go`

### Join Event And Waitlist

1. User joins event.
2. Event row is locked with `FOR UPDATE`.
3. If confirmed registrations are below capacity, user becomes `CONFIRMED`.
4. Otherwise user becomes `WAITLISTED`.
5. Idempotency key and unique constraints prevent duplicate active registration effects.
6. Outbox row records join transition.

Relevant code:

- `internal/services/eventregistration/postgres_repository.go`
- `internal/services/eventregistration/service_test.go`
- `internal/services/eventregistration/postgres_repository_integration_test.go`

### Cancel Registration And Promote Waitlist

1. User/organizer/admin cancels registration.
2. Service validates ownership or elevated role.
3. Transaction cancels target registration.
4. If a confirmed slot opened, first waitlisted user is promoted.
5. Outbox rows capture cancellation and promotion events.

Relevant code:

- `internal/services/eventregistration/service.go`
- `internal/services/eventregistration/postgres_repository.go`
- `internal/services/eventregistration/http.go`

### Feed Projection

1. Outbox relay publishes event/registration messages.
2. Feed worker consumes RabbitMQ messages.
3. Projector checks/stores processed message ID.
4. Feed tables update from event stream.
5. Feed service reads from Postgres and caches response in Redis.

Relevant code:

- `internal/services/feedprojection/projector.go`
- `internal/services/feedprojection/consumer.go`
- `internal/services/feed/service.go`
- `internal/services/feed/cache.go`

### Notification

1. Notification worker consumes relevant events.
2. Consumer stores processed message ID and notification intent in one transaction.
3. RabbitMQ is acked after the intent is durable.
4. Delivery loop claims due intents with a processing lease and sends local SMTP.
5. Notification ledger records `SENT` or `FAILED` delivery state with retry timing.
6. Local Mailpit proves SMTP integration path, not production email guarantees.

Relevant code:

- `internal/services/notification/consumer.go`
- `internal/services/notification/repository.go`
- `internal/services/notification/provider.go`

### Media Upload

1. User requests upload intent.
2. Media service validates metadata and creates DB asset row.
3. MinIO presigned URL is returned.
4. User marks upload complete.
5. Worker claims uploaded assets and checks object existence.
6. Asset becomes `READY` or `FAILED`.

Relevant code:

- `internal/services/media/service.go`
- `internal/services/media/repository.go`
- `internal/services/media/storage.go`

## Security Model

Implemented security controls:

- JWT access tokens with configured TTL of 15 minutes.
- Access token stored in frontend memory, not durable browser storage.
- Rotating refresh tokens in HttpOnly cookies.
- Refresh CSRF protection through readable CSRF cookie plus `X-CSRF-Token` header.
- Refresh token reuse detection revokes token family.
- Access token revocation stored in Postgres and accelerated by Redis cache when enabled.
- RBAC roles: `USER`, `ORGANIZER`, `ADMIN`.
- Admin role update endpoint.
- Organizer/admin moderation for canceling registrations.
- API gateway validates access tokens before protected proxy operations.
- CORS allowlist exists for local/ingress origins.
- Rate limiting middleware supports memory and Redis backends.
- Kubernetes ingress manifest requires TLS secret `cityevents-tls` and SSL redirect.
- Security workflow includes `govulncheck`, `npm audit`, and CodeQL.

Security caveats:

- Local Docker Compose uses development secrets.
- TLS is manifest-ready, not live-certificate proven locally.
- Refresh cookies are secure only when `REFRESH_COOKIE_SECURE=true`; local HTTP keeps it false.
- Access tokens can still be stolen during their TTL if XSS or client compromise occurs.
- No formal penetration test.
- No production secret manager integration.

## Efficiency And Scalability Choices

Implemented optimizations:

- PostgreSQL row locking protects hot event capacity decisions.
- Transactional outbox uses `FOR UPDATE SKIP LOCKED` for concurrent relays.
- RabbitMQ publisher confirms reduce silent message loss after publish attempt.
- Redis cache accelerates feed reads and degrades to Postgres fallback.
- Redis-backed distributed rate limiting is configurable.
- Projection consumers use processed-message tables for dedupe.
- Kubernetes manifests define replicas, probes, resource requests/limits, PDBs, HPA, topology spread, and network policies.
- Vite-built React frontend has static asset serving and runtime `/config.js`, so backend targets can change without rebuilding the bundle.

Performance caveats:

- Load evidence scripts exist but are blocked locally and intended for GitHub Actions only.
- No production load-test report is currently attached.
- Hot-event contention may become a bottleneck because capacity joins serialize on event rows.
- No database read replicas, partitioning, or connection pool tuning evidence.
- No RabbitMQ cluster benchmark.

## DevOps, Guards, And Observability

Implemented:

- Docker Compose local infrastructure.
- Bash start/stop/verify scripts.
- GitHub Actions CI workflow.
- Security workflow.
- Heavy evidence workflow for controlled GitHub Actions execution instead of personal-workstation stress runs.
- Kubernetes manifests and local overlay.
- Ingress manifest for public API gateway entry.
- Readiness/liveness endpoints.
- Prometheus-style `/metrics` endpoint.
- Correlation ID propagation.
- W3C `traceparent` propagation/generation.
- OpenTelemetry collector config.
- Grafana dashboard JSON and Prometheus alert rules for async operations/DLQ/outbox signals.
- Heavy/local-danger scripts guarded by `require_github_actions_evidence_runner`.

Important caveat:

Kubernetes manifests being valid does not prove high availability. HA needs reviewed live-cluster evidence that shows pod replacement, multiple nodes or schedulable failure domains, dependency resilience, ingress reachability, and successful user workflows during/after failure.

## What I Actually Built And Should Be Able To Explain

Use this section as the main study body before practicing questions. If an interviewer asks "what did you personally do?", answer in terms of concrete engineering work, not only technology names.

### Product And Requirements Work

I redefined the project around a clear product: a city event platform where users can browse events, create accounts, publish events as organizers, join events, get waitlisted when capacity is full, and receive eventually consistent feed/notification updates.

The important requirement decisions were:

- Event capacity and waitlist promotion must be strongly consistent.
- Feed, notification, and media processing can be eventually consistent.
- Security must be explicit: roles, JWT, refresh-token handling, CSRF, CORS, rate limiting, and gateway enforcement.
- Resume claims must be evidence-based, not assumed from buzzwords.

Why this matters: architecture is only meaningful if it protects product invariants. In this project, the main invariant is "do not overbook an event and promote the waitlist correctly."

### Backend Implementation Work

I built the backend in Go using small service packages and HTTP handlers rather than a heavy framework. The code uses Go's standard library, Chi routing, explicit service/repository layers, PostgreSQL through pgx, RabbitMQ, Redis, MinIO-compatible object storage, and SMTP/Mailpit for local notification evidence.

The major backend pieces are:

- Auth service: registration, login, short-lived JWT access tokens, rotating refresh tokens, logout/revocation, seeded admin, user role updates.
- API gateway: browser-facing route fanout and protected-route JWT verification.
- Event-registration service: event creation, update/cancel, join, waitlist, cancel join, organizer/admin moderation.
- Outbox relay: reads persisted outbox rows and publishes RabbitMQ messages with retry/dead-state handling.
- Feed projection worker: consumes events and updates query-optimized feed projection tables.
- Feed service: reads projected feed data and uses Redis cache.
- Notification worker/service: consumes event messages, deduplicates processed message IDs, records durable notification intents, and sends local SMTP messages from a separate retryable delivery loop.
- Media service/worker: creates upload intents, presigned URLs, metadata states, and object-existence processing.

### Frontend Implementation Work

I migrated the browser UI to React + TypeScript + Vite so the demo can exercise real product flows without hardcoding backend topology. The frontend reads runtime API targets from `/config.js`, keeps access tokens in memory, uses the HttpOnly refresh-cookie flow, sends CSRF headers for refresh/logout, and gates organizer/admin actions in the UI.

The main frontend pieces are:

- App shell and product views: `frontend/src/App.tsx`
- API client and memory-token behavior: `frontend/src/api.ts`
- View/state helpers: `frontend/src/state.ts`
- Frontend unit tests: `frontend/src/api.test.ts`, `frontend/src/state.test.ts`
- Browser E2E entry: `frontend/playwright.config.mjs`, `frontend/e2e/cityevents.spec.mjs`
- Runtime static server: `frontend/server.mjs`

Tradeoff:

- This is a product/workflow demo, not a production-grade consumer frontend. It does not yet have a full design system, visual regression suite, route-level code splitting, formal accessibility audit, or production CDN/deploy story.

### Architecture Thinking Practiced

The architecture practices demonstrated are:

- Service boundary design: event and registration are fused because they share one consistency boundary.
- Synchronous vs asynchronous split: critical registration state is synchronous; feed/notification/media are async.
- Transactional consistency: PostgreSQL transactions and row locks protect capacity/waitlist behavior.
- Reliable event publishing: transactional outbox avoids the classic DB-commit/publish-loss gap.
- At-least-once messaging: RabbitMQ redelivery is expected and consumers are made idempotent.
- Failure isolation: async workers can lag or fail without breaking the core join workflow.
- Operational honesty: Kubernetes manifests are not treated as proof of production HA.

### Robustness Thinking Practiced

The robustness choices were:

- Use database transactions for event mutation correctness.
- Use `FOR UPDATE` locks for capacity and waitlist operations.
- Use idempotency keys for join operations.
- Use processed-message tables for consumer deduplication.
- Use outbox `FAILED` and `DEAD` states for publish failures.
- Use RabbitMQ retry and DLQ routing for poison consumer messages.
- Use readiness/liveness probes in Kubernetes manifests.
- Use scripts and CI gates to make claims repeatable.

The limits are equally important:

- Idempotency does not make the queue exactly-once.
- DLQs do not fix failed business logic; they preserve failed work for inspection.
- Retries can amplify load during outages if not bounded.
- Row locking protects correctness but can reduce throughput for hot events.

### Security Thinking Practiced

The security design is layered:

- Authentication: JWT access token plus refresh-token session.
- Token lifetime: access token is short-lived; refresh token is longer-lived but rotated.
- Storage choice: access token stays in frontend memory; refresh token is HttpOnly cookie.
- CSRF protection: refresh/logout cookie flows require a CSRF header.
- Authorization: role model separates `USER`, `ORGANIZER`, and `ADMIN`.
- Gateway boundary: protected browser-facing operations require JWT verification.
- Abuse control: rate limiting supports Redis-backed distributed limits.
- Vulnerability checks: Go and npm dependency scans are part of the verification path.
- TLS intent: Kubernetes ingress requires a TLS secret; live cert-manager issuance is future evidence.

Interview-safe phrasing:

> I reduced token theft impact with short-lived in-memory access tokens and rotating HttpOnly refresh tokens. I still consider XSS and infrastructure compromise real risks, so this is hardening, not a perfect guarantee.

### DevOps Thinking Practiced

The DevOps and evidence design includes:

- Docker Compose for local dependencies.
- Bash scripts for phase verification and local start/stop workflows.
- GitHub Actions CI for repeatable checks.
- Security workflow for dependency and code scanning.
- Kubernetes manifests for deployment readiness.
- Ingress, probes, resources, replicas, PDBs, HPA, topology spread, network policies.
- Heavy-evidence scripts guarded so load tests and Minikube failure tests run in controlled GitHub Actions, not on a personal workstation.

Interview-safe phrasing:

> I made the project manifest-ready and CI-checked, but I would not claim production deployment or proven HA until the live cluster evidence is run and reviewed.

### Testing And Engineering Discipline Practiced

Do not claim strict TDD unless you can show a test-first commit history. The safer claim is:

> I used test-driven acceptance gates: define the behavior and exit criteria, implement unit/integration/smoke tests around the risky behavior, and keep phase verification scripts as the evidence boundary.

Concepts practiced:

- Unit testing for domain rules, token logic, cache/revocation behavior, and HTTP handlers.
- Integration testing for PostgreSQL, RabbitMQ, Redis, MinIO, and Mailpit-backed flows.
- Concurrency testing for joins, idempotency, relay locking, and worker claims.
- Smoke testing for service runtime readiness and key endpoint workflows.
- Security testing through auth/RBAC/CSRF/revocation tests and dependency scans.
- Static infrastructure validation through Docker Compose and Kubernetes kustomize checks.
- Regression fixing when verification scripts exposed false assumptions.

If asked "was it TDD?":

> Not strict red-green-refactor for every line. I treated correctness requirements as testable acceptance gates, especially around concurrency, async delivery, idempotency, and security. For future work, I would make strict TDD more explicit by opening failing tests first and preserving that evidence in commits.

### Software Engineering Concepts This Project Exercises

Core concepts:

- Domain modeling: events, registrations, roles, media assets, notifications.
- API design: route structure, HTTP status handling, validation, idempotency headers.
- Consistency models: strong consistency for writes, eventual consistency for projections.
- Distributed systems: at-least-once delivery, retries, DLQs, outbox, idempotency.
- Concurrency control: row locks, unique constraints, `SKIP LOCKED`, bounded retries.
- Security: authentication, authorization, token lifecycle, CSRF, CORS, rate limiting.
- Observability: metrics, correlation IDs, trace context propagation, debug runbooks.
- DevOps: containers, CI, Kubernetes manifests, probes, resource limits, guarded evidence.
- Testing strategy: unit, integration, smoke, E2E readiness, static checks, security scans.
- Tradeoff analysis: monolith vs microservices, RabbitMQ vs Kafka/SQS, Redis cache vs source of truth, manifest-ready vs production-proven.

The strongest interview angle is not "I used many technologies." It is:

> I designed the system around correctness boundaries. I kept critical event registration synchronous and transactional, then moved non-critical fanout work to asynchronous, idempotent projections.

## Engineering Thinking Study Map

This section is the deeper interview-prep layer. The point is to explain not just what was used, but what engineering judgment each part demonstrates.

A strong answer should usually follow this shape:

1. State the product invariant or failure mode.
2. Explain the design choice.
3. Explain why a simpler or alternative choice was not selected.
4. Admit the tradeoff.
5. Say how the project verifies it.
6. Say what remains unproven.

### 1. Requirements Before Architecture

Engineering concept: requirements drive architecture.

The main product invariant is:

> An event must not be overbooked, and waitlist promotion must be deterministic.

That invariant is more important than whether the system looks like a textbook microservices diagram. This is why event creation, joining, waitlisting, cancellation, and promotion are kept in the same service and database transaction.

What this demonstrates:

- I can separate core correctness from supporting features.
- I do not split services just because the nouns are different.
- I can explain why some data must be strongly consistent while other data can be eventually consistent.

Alternative considered:

- Split event catalog and registration into separate services.

Why not:

- A join operation would need to check event state, capacity, registration state, and waitlist ordering across service boundaries.
- That pushes the design toward sagas, distributed locks, or compensating actions for something that is naturally one transaction.

Tradeoff accepted:

- The event-registration service becomes larger.
- It owns more domain logic.
- Scaling that service is more important because it is on the critical path.

Interview line:

> I treated service boundaries as consistency boundaries, not database-table boundaries.

Study links:

- [Transactional outbox pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- [PostgreSQL transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html)

### 2. Consistency Boundary And Database Transactions

Engineering concept: some operations need ACID semantics.

In CityEvents, joining an event is not just "insert a row." The operation must:

- Check event exists and is not canceled.
- Count current confirmed registrations.
- Decide confirmed vs waitlisted.
- Prevent duplicate active registrations.
- Preserve waitlist order.
- Emit a domain event through the outbox.

Why PostgreSQL is the right source of truth here:

- It supports transactions.
- It supports row locks.
- It supports unique constraints.
- It gives durable state that can rebuild projections.

What I did:

- Used PostgreSQL transactions for event mutation paths.
- Used row locking to serialize conflicting joins/cancellations for the same event.
- Used idempotency keys and constraints to reduce duplicate effects.
- Wrote integration/concurrency tests around join/waitlist behavior.

Alternative considered:

- Use Redis counters for capacity.

Why not:

- Redis counters are fast, but then the source of truth is split between Redis and Postgres.
- Handling cancel, waitlist promotion, replay, and recovery becomes harder.

Tradeoff accepted:

- PostgreSQL row locks are correct but can become a hot-event bottleneck.

How to improve later:

- Queue admission for extremely hot events.
- Optimistic concurrency with bounded retries.
- Preallocated capacity tokens.
- Partitioning or sharding by event ID.

Interview line:

> I chose transactional correctness first, then identified hot-event throughput as the scaling tradeoff.

Study links:

- [PostgreSQL explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html)
- [PostgreSQL transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html)

### 3. Synchronous Core, Asynchronous Edges

Engineering concept: latency and availability improve when non-critical work is decoupled.

The user must immediately know whether joining succeeded. But the feed, notifications, and media post-processing do not need to block that join response.

What I did:

- Kept registration writes synchronous.
- Moved feed projection and notifications to RabbitMQ workers.
- Treated feed/notification state as eventually consistent.

Why:

- If notification delivery is slow, joining an event should still work.
- If feed projection lags, the source-of-truth registration state remains correct.
- Async workers can be retried, scaled, or paused without directly breaking the critical write.

Alternative considered:

- Directly update feed and send notification inside the join transaction.

Why not:

- That increases latency.
- It couples user-facing writes to external systems.
- It makes failures harder: if email fails, should the join fail?

Tradeoff accepted:

- Users may briefly see stale feed/notification state.
- Debugging requires tracing async paths.

Interview line:

> I kept the command path strongly consistent and made read/fanout paths eventually consistent.

Study links:

- [RabbitMQ consumer acknowledgements and publisher confirms](https://www.rabbitmq.com/docs/confirms)
- [Transactional outbox pattern](https://microservices.io/patterns/data/transactional-outbox.html)

### 4. Transactional Outbox As Reliability Boundary

Engineering concept: avoid dual-write bugs.

The dangerous naive flow is:

1. Write event registration to Postgres.
2. Publish message to RabbitMQ.

Failure case:

- If Postgres commits but the process crashes before RabbitMQ publish, the registration exists but feed/notification never hear about it.

What I did:

- Wrote `outbox_messages` in the same transaction as the domain change.
- Built an outbox relay that reads available rows.
- Published to RabbitMQ with confirms.
- Marked rows `SENT`, `FAILED`, or `DEAD`.

Alternative considered:

- Publish directly from the HTTP handler.

Why not:

- Simpler code, but weaker reliability.

Alternative considered:

- CDC with Debezium.

Why not yet:

- More production-grade for some systems, but adds Kafka/connector infrastructure and operational complexity.

Tradeoff accepted:

- More tables and relay code.
- Eventual consistency latency.
- Need retention/cleanup policy for outbox rows.

Interview line:

> The outbox is my boundary against the database-plus-broker dual-write problem.

Study links:

- [Transactional outbox pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- [RabbitMQ publisher confirms](https://www.rabbitmq.com/docs/confirms)

### 5. Delivery Semantics: At-Least-Once vs Exactly-Once

Engineering concept: delivery guarantees and business effects are different.

RabbitMQ can redeliver messages. This is normal. A consumer can:

1. Receive message.
2. Write projection to Postgres.
3. Crash before acknowledging the broker.
4. Restart and receive the same message again.

That is why "exactly-once consumption" is the wrong literal claim.

What I did:

- Used stable message IDs.
- Stored processed message IDs in projection/notification tables.
- Made duplicate delivery safe at the business layer.

What this proves:

- Duplicate messages should not duplicate projection effects.

What this does not prove:

- The broker only delivered once.
- The handler only executed once.
- All possible side effects are exactly-once.

How to talk about SQS FIFO:

- AWS SQS FIFO describes exactly-once processing in the sense of deduplication within its model and deduplication window.
- You still need idempotent side effects when your consumer writes to an external database or calls an external provider.

How to talk about Kafka:

- Kafka has exactly-once semantics for certain producer/transaction/stream-processing workflows.
- That does not automatically make arbitrary external side effects exactly-once.

Interview line:

> I do not claim exactly-once delivery. I claim at-least-once delivery with idempotent consumers and effectively-once business effects for the projections I control.

Study links:

- [RabbitMQ acknowledgements and confirms](https://www.rabbitmq.com/docs/confirms)
- [Amazon SQS FIFO exactly-once processing](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues-exactly-once-processing.html)
- [Apache Kafka message delivery semantics](https://kafka.apache.org/documentation/#semantics)

### 6. Retry, Backoff, And Dead Lettering

Engineering concept: retries need limits and observability.

Retries are useful for transient failures. They are dangerous when the error is permanent.

What I did:

- Added RabbitMQ retry and DLQ topology for consumer failures.
- Added outbox retry states for publish failures.
- Added terminal `DEAD` state for outbox rows that exceed attempts.
- Added scripts and dashboards/alerts for inspecting async failure state.

Why:

- Without retry, transient broker/worker failures lose work.
- Without a DLQ/dead state, poison messages retry forever.
- Without visibility, failures silently accumulate.

Tradeoff accepted:

- Retrying increases message latency.
- Aggressive retries can amplify outages.
- DLQs require operational ownership.

Interview line:

> Retry is not reliability by itself. Retry plus bounded attempts, DLQs, metrics, and a replay policy is reliability engineering.

Study links:

- [RabbitMQ dead letter exchanges](https://www.rabbitmq.com/docs/dlx)
- [RabbitMQ message TTL](https://www.rabbitmq.com/docs/ttl)

### 7. Read Models, Caching, And Redis

Engineering concept: cache is an optimization, not a source of truth.

The feed is a read model. It is designed for query convenience, not for primary consistency.

What I did:

- Feed workers project events/registrations into feed tables.
- Feed service reads those tables.
- Redis caches feed responses.
- If Redis fails, feed can fall back to Postgres.

Why Redis fits:

- Fast reads.
- Shared distributed rate-limit counters.
- Short-lived revocation acceleration.

Why Redis should not own critical state here:

- Cache loss should not corrupt event registration.
- Redis persistence/replication would need separate production hardening.

Tradeoff accepted:

- Feed can be stale.
- Cache invalidation/TTL decisions affect freshness.
- Redis outage can reduce performance or affect rate-limit behavior.

Interview line:

> Redis is deliberately non-authoritative. Postgres remains the recovery source.

Study links:

- [Redis documentation](https://redis.io/docs/latest/)
- [Redis distributed locks and safety discussion](https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/)

### 8. Security Threat Modeling

Engineering concept: security controls should map to threats.

Threat: stolen access token.

- Control: short TTL and in-memory frontend storage.
- Limitation: active token remains usable until expiry unless revoked.

Threat: stolen refresh token.

- Control: HttpOnly cookie, rotation, family revocation on reuse.
- Limitation: device/browser compromise is still serious.

Threat: CSRF on cookie-backed refresh/logout.

- Control: CSRF cookie plus `X-CSRF-Token` header.
- Limitation: XSS can undermine CSRF defenses.

Threat: unauthorized role/action.

- Control: JWT verification and RBAC checks.
- Limitation: every protected handler must consistently enforce authorization.

Threat: abuse/brute force.

- Control: login guard and rate limiting.
- Limitation: serious production abuse needs WAF, IP reputation, monitoring, and account protection.

Threat: dependency vulnerability.

- Control: `govulncheck`, `npm audit`, CodeQL.
- Limitation: scanners do not replace code review or threat modeling.

Interview line:

> I think of security as layered risk reduction, not a single feature. CORS, CSRF, JWT, and RBAC solve different problems.

Study links:

- [OWASP JWT cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html)
- [OWASP CSRF prevention cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [OWASP XSS prevention cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)
- [MDN CORS guide](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS)

### 9. API Gateway And BFF Thinking

Engineering concept: gateway boundaries simplify clients but can become a bottleneck.

The gateway is the browser-facing entry point. It routes to internal services and centralizes some cross-cutting concerns.

What this demonstrates:

- I can hide internal service topology from the frontend.
- I can centralize auth checks for protected browser flows.
- I can make Kubernetes ingress point at one public API surface.

Alternative:

- Frontend calls every service directly.

Why not:

- More CORS config.
- More public services.
- More duplicated auth logic.
- Harder future API evolution.

Tradeoff:

- Gateway can become a bottleneck.
- Gateway needs good timeouts, retries, and error handling.
- Too much logic in gateway can become a mini-monolith.

Interview line:

> The gateway is a boundary for public API shape and security enforcement, not a place to hide domain logic.

### 10. Kubernetes And HA Thinking

Engineering concept: orchestration intent is not production proof.

What I added:

- Deployments for services/workers.
- Services for networking.
- Ingress for public API path.
- Readiness/liveness probes.
- Resource requests/limits.
- Replicas.
- PodDisruptionBudgets.
- HPA.
- Topology spread.
- Network policies.
- TLS ingress intent.

What this demonstrates:

- I understand the primitives required for a deployable service.
- I know Kubernetes can restart and route around unhealthy pods only if probes/resources/replicas are configured.

What it does not prove:

- The cluster is live.
- Images are deployed.
- Multi-node scheduling works.
- Dependencies are HA.
- The app survives real node/broker/database failures.

Interview line:

> Kubernetes YAML is deployment intent. HA is a property proven by running failure tests in a real environment.

Study links:

- [Kubernetes probes](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-probes)
- [Kubernetes ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)
- [Kubernetes horizontal pod autoscaling](https://kubernetes.io/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/)

### 11. Observability And Debuggability

Engineering concept: if async systems fail, logs alone are not enough.

What I added:

- Correlation ID propagation.
- W3C `traceparent` propagation/generation.
- Structured logs.
- Prometheus-style metrics.
- Async operation dashboard/alert config.
- Debugging walkthrough docs.

Why:

- Async failures cross process boundaries.
- A user action may create DB state, outbox rows, broker messages, worker logs, and projection records.
- Debugging needs a way to connect those artifacts.

Limitation:

- Real OpenTelemetry spans and a trace backend are not fully proven yet.

Interview line:

> I designed the debugging path around following one user action across HTTP, outbox, RabbitMQ, worker, and projection state.

Study links:

- [OpenTelemetry traces](https://opentelemetry.io/docs/concepts/signals/traces/)
- [RabbitMQ monitoring guide](https://www.rabbitmq.com/docs/monitoring)

### 12. Testing Strategy As Evidence

Engineering concept: tests should match risk.

The testing strategy is not "write tests because tests are good." It is:

- Domain tests for business rules.
- HTTP handler tests for API and auth behavior.
- Integration tests for Postgres/RabbitMQ/Redis/MinIO/Mailpit.
- Concurrency tests where race conditions matter.
- Smoke scripts for runtime wiring.
- Security scans for known dependency/code risks.
- Static Kubernetes checks for manifest validity.
- Heavy evidence scripts for load/failure tests, guarded to run in GitHub Actions.

What this demonstrates:

- I can map test type to failure mode.
- I understand that unit tests cannot prove broker or database behavior.
- I understand that static Kubernetes validation cannot prove live HA.

Tradeoff:

- More test infrastructure takes time to maintain.
- Integration tests are slower and can be flaky if shared state is not controlled.

Interview line:

> My verification strategy follows the risk: transaction logic gets integration/concurrency tests, security gets auth/CSRF/RBAC tests, and infrastructure gets static plus future live evidence.

Study links:

- [Go testing package](https://pkg.go.dev/testing)
- [How to write Go code](https://go.dev/doc/code)
- [Effective Go](https://go.dev/doc/effective_go)

### 13. Simplicity And Over-Engineering Judgment

Engineering concept: senior judgment is knowing what not to distribute.

A harsh interviewer may say this project is too heavy for an event app. The best answer is to agree partially:

> For a real startup MVP, a modular monolith might be better. For this project, the goal is to demonstrate distributed-system thinking. I still avoided unnecessary distribution in the most sensitive place by keeping event and registration together.

This answer shows judgment because it does not defend every choice as universally optimal.

Architecture choices by context:

| Scenario | Better choice |
| --- | --- |
| Fast MVP, one team, low scale | Modular monolith |
| Learning/resume distributed systems | Small microservices with explicit boundaries |
| High-throughput immutable stream analytics | Kafka-style event log |
| Simple managed queue with cloud integration | SQS/SNS or managed broker |
| Strong booking/payment consistency | Single transactional boundary plus provider idempotency |

Interview line:

> I can defend the architecture as a learning/resume system, but I would choose a simpler deployment if the only goal were shipping an MVP quickly.

### 14. Production Readiness Gap Analysis

Engineering concept: production readiness is evidence, not intention.

Current evidence:

- Unit/integration/smoke tests pass.
- Security scans pass.
- Static Kubernetes checks pass.
- Local Docker environment works.
- React + TypeScript frontend builds and is served by the local startup script.
- Local full stack responds at `http://127.0.0.1:18088`, gateway `/readyz` is healthy, and seeded-admin login returns `ADMIN`.
- Heavy evidence scripts are guarded.

Still missing:

- Live staging deployment.
- Managed database/broker/cache.
- Secret manager.
- Real TLS issuance.
- Real OpenTelemetry traces.
- Load-test report.
- Failure-test report.
- Backup/restore test.
- Runbook for DLQ replay and outbox cleanup.
- Error budget/SLO definition.
- Production frontend hosting/CDN path, visual regression checks, and accessibility audit.

Interview line:

> I separate built, configured, and proven. This project is built and statically deployment-ready, but production HA and throughput remain unproven until live evidence is collected.

### Study Order

If you need to study efficiently, use this order:

1. PostgreSQL transactions, row locks, and isolation.
2. Transactional outbox and dual-write failure.
3. RabbitMQ acknowledgements, publisher confirms, retries, DLQs.
4. Idempotency and effectively-once business effects.
5. JWT, refresh tokens, CSRF, CORS, XSS.
6. Redis cache vs source of truth.
7. Kubernetes probes, ingress, HPA, PDBs, and why HA needs live failure evidence.
8. OpenTelemetry traces, correlation IDs, metrics, and debugging async workflows.
9. Go idioms, explicit errors, interfaces, package boundaries, and test strategy.

## Claim Verification Table

| Claim | Status | Evidence | Resume-safe wording |
| --- | --- | --- | --- |
| Microservices architecture | Supported | Separate Go commands under `cmd/*`, independent service packages, Docker/K8s deployments | "Built a Go microservices event platform..." |
| API gateway entry point | Supported | `internal/services/gateway`, Kubernetes ingress routes to gateway | "API gateway centralizes browser-facing routes." |
| Async queue decoupling | Supported | Outbox relay, RabbitMQ topology, feed/notification workers | "Decoupled feed/notification projections through RabbitMQ." |
| Message will always be stored | Partially supported | Transactional outbox stores domain event with DB transaction | "Outbox reduces lost-publish risk for committed domain changes." |
| Exactly-once consumption | Not supported as literal claim | RabbitMQ at-least-once plus processed-message dedupe | "Effectively-once projection effects through idempotent consumers." |
| DLQ and retry | Supported | RabbitMQ retry exchanges/DLQs and outbox `FAILED`/`DEAD` states | "Retry and dead-letter handling for poison async work." |
| High throughput | Not proven | Load scripts exist but local run intentionally blocked | "Designed for throughput with async workers and caching; load evidence pending." |
| High availability | Manifest-ready, not proven | Replicas, PDBs, HPA, probes, topology spread; no live cluster proof | "Kubernetes-ready manifests with HA primitives; live HA proof pending." |
| Redis optimization | Supported | Feed cache, rate limiting, token revocation cache | "Redis used for cache, rate limiting, and revocation acceleration." |
| Security/RBAC | Supported with caveats | JWT, refresh cookies, CSRF, roles, gateway auth, tests | "Implemented JWT/RBAC and rotating refresh-token security model." |
| React + TypeScript frontend | Supported as product demo | `frontend/src/App.tsx`, `frontend/src/api.ts`, Vite build, runtime `/config.js` | "Built a React/TypeScript frontend for event browsing, auth, publishing, joining, moderation, and admin role workflows." |
| CI/CD | Supported for CI, not deployment CD | GitHub Actions CI/security/heavy-evidence workflows | "CI/security gates with deploy evidence pending." |

## Verification Run Summary

Available verifier scripts passed locally on 2026-06-13 and were kept aligned with later changes:

- `bash ./scripts/verify-phase-1.sh`
- `bash ./scripts/verify-phase-2.sh`
- `bash ./scripts/verify-phase-3.sh`
- `bash ./scripts/verify-phase-4.sh`
- `bash ./scripts/verify-phase-5.sh`
- `bash ./scripts/verify-phase-6.sh`
- `bash ./scripts/verify-phase-7.sh`
- `bash ./scripts/verify-phase-8.sh`
- `bash ./scripts/verify-phase-9.sh`
- `bash ./scripts/verify-phase-15.sh`

`verify-phase-15.sh` chains the static gates for phases 10 to 14 and confirms that live Kubernetes/heavy evidence remains intentionally skipped unless run through GitHub Actions.

Extra checks passed:

- `govulncheck ./...`
- `npm audit --omit=dev --audit-level=high`
- `docker compose config --quiet`
- `kubectl kustomize` and client dry-run through the Phase 10/15 path
- `npm run verify` for the React + TypeScript frontend during the frontend migration

Fresh local runtime evidence on 2026-06-15:

- `bash ./scripts/verify-phase-8.sh` passed: `go test ./...`, frontend TypeScript typecheck, 14 frontend unit tests, and Vite production build.
- `./scripts/start-local.sh --frontend-port 18088` started Docker dependencies, Go services/workers, and the Vite-built frontend. The shell command timed out after five minutes, but the processes were listening and health checks passed.
- `GET http://127.0.0.1:18088/` returned `200`.
- `GET http://127.0.0.1:8080/readyz` returned gateway status `ok`.
- `POST http://127.0.0.1:8080/v1/auth/login` with `admin@cityevents.local` / `AdminPass12345` returned role `ADMIN`.

Verification fixes made during this audit:

- Phase 1 startup checks now avoid accidentally leaving HTTP services bound on local ports.
- Phase 3, 5, and 7 runtime smoke scripts now use Windows-compatible `curl.exe`/process-launch behavior under Git Bash.
- Phase 5 and 6 integration scopes now match their actual dependencies instead of running unrelated MinIO/Mailpit tests.
- Phase 7 and 9 integration suites run serially to avoid schema-reset races.
- Outbox dead-state integration test now primes `available_at` in the past to avoid clock-bound flakiness.

## Remaining Improvements

Highest-value next work:

1. Run heavy evidence in GitHub Actions and attach generated summaries: load, dependency failure, Kubernetes live smoke.
2. Add real OpenTelemetry SDK spans and verify traces in a collector/backend.
3. Add managed Postgres/RabbitMQ/Redis deployment plan or Terraform/Pulumi staging environment.
4. Add production secret management and cert-manager-backed TLS proof.
5. Add outbox cleanup/retention policy and admin DLQ replay tooling with audit trails.
6. Add frontend visual regression, accessibility checks, route-level structure, and production static hosting/CDN plan.
7. Add load-test targets and acceptance thresholds to resume claims only after evidence exists.

## Resume Claim Boundary

Use this:

> Built CityEvents, a React + TypeScript and Go microservices platform for local event publishing and registration. The system uses PostgreSQL transactions for event capacity and waitlist correctness, a transactional outbox with RabbitMQ for asynchronous feed/notification projections, Redis for caching/rate limiting/token revocation acceleration, JWT/RBAC with rotating HttpOnly refresh tokens, and Kubernetes-ready manifests with probes, replicas, PDBs, HPA, ingress, and security policies. Verified with unit, integration, smoke, frontend, security, and static Kubernetes checks.

Avoid this until future evidence exists:

> Exactly-once consumption, guaranteed message storage under all failures, production high availability, production deployment, payment-grade side-effect prevention, or proven high throughput.

## Harsh Interviewer Critique

Use these critiques to pressure-test the resume. If you cannot answer these cleanly, the claim should be weakened or the system should be improved.

### Critique 1: "This Looks Over-Engineered"

Attack:

> For a city event app, microservices, RabbitMQ, Redis, Kubernetes, and outbox look excessive. Why not build a monolith?

Good answer:

> For a real MVP, a modular monolith would probably be the most efficient. I chose microservices because the project goal was to practice distributed-system boundaries. I still kept the most coupled domain, event plus registration, fused into one service because splitting it would make capacity and waitlist correctness harder. So the architecture is intentionally resume/learning-oriented, but the boundaries are not random.

Weak answer to avoid:

> Microservices are always more scalable.

### Critique 2: "You Claimed Exactly-Once. That Is Wrong."

Attack:

> RabbitMQ does not guarantee exactly-once consumption. Explain the failure case.

Good answer:

> Correct. Literal exactly-once consumption is not what I should claim. If a consumer writes to Postgres and crashes before acknowledging RabbitMQ, RabbitMQ can redeliver the same message. The system handles this with idempotent consumers by storing processed message IDs. So the accurate claim is at-least-once delivery with effectively-once business effects for feed projections and notification-intent records, while external email delivery still depends on provider idempotency.

Weak answer to avoid:

> RabbitMQ persistent queues mean exactly once.

### Critique 3: "Message Will Always Be Stored Is Too Strong"

Attack:

> What happens if Postgres is down, the disk is full, or the transaction never commits?

Good answer:

> Then the message is not stored. The claim only applies after the domain transaction successfully commits. The transactional outbox means if an event registration change commits, the corresponding outbox row commits with it. It does not mean every attempted request survives every infrastructure failure.

### Critique 4: "Kubernetes Does Not Mean High Availability"

Attack:

> You put it on Kubernetes. So what? Did you prove HA?

Good answer:

> Not production HA. The manifests include HA primitives like replicas, probes, PDBs, HPA, topology spread, and network policies, but that is manifest readiness. Real HA proof needs a live cluster test showing pod failure/replacement, dependency availability, ingress reachability, and successful workflows during failure. That evidence is intentionally marked pending.

### Critique 5: "Your Hot Event Join Path Will Bottleneck"

Attack:

> If 10,000 people join one event at once, your row lock serializes them.

Good answer:

> Yes, that is the tradeoff. Row locking gives a simple correctness guarantee for capacity and waitlist order, but it can bottleneck hot events. To scale that path, I would consider preallocated capacity tokens, queue-based admission, event sharding, or optimistic concurrency with retry/backoff. For this version, correctness was prioritized over maximum write throughput.

### Critique 6: "Redis Can Lose Data"

Attack:

> You use Redis. What happens if Redis is flushed?

Good answer:

> Redis is not the source of truth in this design. Feed cache can be rebuilt from Postgres projections. Rate limiting becomes degraded depending on fail-open/fail-closed config. Revocation cache accelerates checks, but Postgres remains the durable revocation source. If Redis loss destroys critical state, the design would be wrong.

### Critique 7: "Your Security Is Not Complete"

Attack:

> Access tokens can still be stolen. CSRF does not stop XSS. CORS is not authentication.

Good answer:

> Agreed. The design reduces risk but does not eliminate it. Access tokens are short-lived and kept in memory to reduce persistence risk. Refresh tokens are HttpOnly and rotated. CSRF protects cookie-authenticated refresh/logout flows, while CORS limits browser origins but is not an auth boundary. XSS still matters, so production would need strict CSP, dependency hygiene, output escaping, and security review.

### Critique 8: "Your Observability Is Mostly Static"

Attack:

> Do you actually have distributed tracing or just trace headers?

Good answer:

> Currently the project propagates correlation IDs and W3C trace context, exposes metrics, and includes an OpenTelemetry collector config. Full span instrumentation and trace-backend evidence are not complete. I would describe it as traceability groundwork, not fully proven distributed tracing.

### Critique 9: "CI/CD Is Not Deployment"

Attack:

> You say CI/CD. Where is the CD?

Good answer:

> The accurate claim is CI and deployment readiness, not full CD. The GitHub Actions workflows run tests, security checks, browser checks, build checks, and heavy evidence workflows. I do not have an automated production deployment pipeline yet.

### Critique 10: "Frontend Is Not Production Grade"

Attack:

> Your backend is heavy but the frontend is still not production-grade. Why?

Good answer:

> The frontend is now React + TypeScript with Vite, runtime gateway configuration, memory-only access tokens, refresh-cookie support, and role-aware workflows. I still treat it as a product demo and workflow verifier, not the main production claim. If this became customer-facing, I would add a stronger route/component structure, accessibility checks, visual regression, performance budgets, and a production static-hosting/CDN path.

## Interview Question Bank And Prepared Answers

Practice answering aloud. Do not memorize word-for-word; memorize the mechanism.

### 1. Give me the 60-second project overview.

Answer:

> CityEvents is a Go microservices event platform. Users can register, browse events, organizers can publish events, attendees can join events, and the system handles capacity, waitlists, feed projection, notifications, and media upload metadata. The key design is that event registration is strongly consistent in PostgreSQL, while feed and notifications are asynchronous through a transactional outbox and RabbitMQ. I used Redis for cache/rate limiting/revocation acceleration, JWT/RBAC for security, and Kubernetes manifests plus CI scripts for operational readiness.

### 2. What was the hardest technical problem?

Answer:

> The hardest part was separating what must be strongly consistent from what can be eventually consistent. Event capacity and waitlist promotion cannot be approximate, so those happen in one PostgreSQL transaction with row locks. But feed and notification fanout do not need to block the user, so those are published through an outbox and processed asynchronously with idempotent consumers.

### 3. Why did you use Go?

Answer:

> Go fits this project because it is good for small network services: fast startup, simple deployment as static-ish binaries, strong standard library, explicit error handling, good concurrency primitives, and mature ecosystem for HTTP, Postgres, Redis, and RabbitMQ. The tradeoff is that Go is more verbose than something like Node/Python for CRUD, and without discipline the code can become repetitive. I accepted that for clarity and operational simplicity.

### 4. Why no heavy Go framework?

Answer:

> I used a lightweight approach with Chi and explicit service/repository layers because I wanted the architecture and failure boundaries to stay visible. A full framework could speed up CRUD, but it can hide middleware and lifecycle behavior. For interview purposes, explicit code makes auth, retries, transactions, and observability easier to explain.

### 5. Why RabbitMQ instead of Kafka?

Answer:

> RabbitMQ is a better fit for this project because I need durable task/event delivery to a small number of workers, retry routing, DLQs, and simpler local operations. Kafka is better if I need high-throughput immutable event streams, partitioned replay, and long retention. The tradeoff is RabbitMQ gives at-least-once delivery and less replay-oriented architecture, so I handle duplicates with idempotent consumers.

### 6. Explain the transactional outbox in this project.

Answer:

> When the event-registration service changes domain state, it writes an outbox row in the same PostgreSQL transaction. The outbox relay later reads available rows using locking, publishes them to RabbitMQ with confirms, and marks them sent. If publish fails, the row becomes failed with backoff, and eventually dead. This avoids losing a message when the DB commit succeeds but the app crashes before publishing.

### 7. Does your system guarantee exactly-once?

Answer:

> No. It guarantees neither literal exactly-once delivery nor literal exactly-once consumption. It uses at-least-once messaging with idempotent handlers. The business goal is effectively-once side effects for data the system controls, achieved by processed-message tables and idempotent domain operations. External side effects such as email require provider idempotency or reconciliation and are not claimed as strict exactly-once.

### 8. What happens if RabbitMQ goes down?

Answer:

> Core event registration can still commit because the outbox row is stored in Postgres. The relay fails to publish and marks retryable failure or leaves work pending depending on the failure point. Feed and notification projections lag until RabbitMQ returns. The user-facing critical write path remains protected, but async views become stale.

### 9. What happens if Postgres goes down?

Answer:

> Critical writes fail. That is expected because Postgres is the source of truth. The system should return errors rather than pretending success. For production, I would use managed Postgres or a properly operated HA Postgres setup with backups, PITR, monitoring, and tested failover.

### 10. What happens if Redis goes down?

Answer:

> Redis-backed features degrade depending on the feature. Feed can fall back to Postgres. Rate limiting can be configured fail-open or fail-closed. Revocation cache falls back to the durable Postgres revocation source. Redis is an optimization layer, not the primary state store.

### 11. How does waitlist promotion work?

Answer:

> When a confirmed attendee cancels, the service runs a transaction, cancels that registration, finds the earliest waitlisted registration using ordered selection and locking, promotes it to confirmed, and writes outbox messages for the transition. This keeps capacity and promotion order consistent.

### 12. What race conditions did you think about?

Answer:

> Concurrent joins against the same event, duplicate join requests from the same user, concurrent outbox relays selecting the same row, duplicate RabbitMQ deliveries, and concurrent media workers claiming the same uploaded asset. The system uses row locks, unique/idempotency constraints, `SKIP LOCKED`, and processed-message tables to handle those.

### 13. How do you protect against duplicate messages?

Answer:

> I do not try to prevent duplicate delivery at the broker layer. I make consumers idempotent. Each message has a stable message ID, and consumers record processed message IDs before or with the projection effect. If the same message arrives again, the consumer can skip or no-op it.

### 14. What is the difference between idempotency and exactly-once?

Answer:

> Exactly-once means the operation happens only once at the delivery or execution level. Idempotency means the operation can be retried multiple times but the final business state is the same as if it happened once. This project relies on idempotency because exactly-once across broker acknowledgement and database writes is not generally achievable without a shared transaction system.

### 15. Why are feed and notifications eventually consistent?

Answer:

> They are not part of the critical user decision. If a user joins an event, the join result must be correct immediately. But feed cards and notification records can lag by seconds. Making them async reduces latency and decouples failure in non-critical fanout from the core registration transaction.

### 16. What did you do for rate limiting?

Answer:

> The HTTP middleware supports scoped limits for auth, mutation, and read requests. It can use memory locally or Redis for distributed rate limiting. Metrics record rate-limited requests and store errors. The tradeoff is fail-open protects availability but weakens abuse protection; fail-closed protects abuse control but can block valid traffic during Redis failure.

### 17. How does JWT revocation work?

Answer:

> Access tokens include an ID. On logout, the token ID is stored as revoked in Postgres until expiry, and Redis can cache that revocation. During authenticated requests, the auth path checks whether the token ID has been revoked. This gives logout semantics even though JWTs are normally stateless.

### 18. Why use refresh-token rotation?

Answer:

> A long-lived static refresh token is high risk if stolen. Rotation means every refresh returns a new token and invalidates the old one. If an old token is reused, that suggests theft or replay, so the refresh family can be revoked.

### 19. Why CSRF if you use JWT?

Answer:

> The access token is sent as a bearer token, but refresh and logout involve cookies. Browser cookies can be sent automatically cross-site, so those cookie-backed operations need CSRF protection. CORS is not enough because CORS is a browser read/access policy, not a complete request-forgery defense.

### 20. What does CORS protect?

Answer:

> CORS controls which browser origins can read responses and send certain credentialed requests. It is useful but not an authentication mechanism. Non-browser clients can ignore CORS, so the server still needs JWT validation, CSRF for cookie flows, and authorization checks.

### 21. What is your Kubernetes entry point?

Answer:

> The intended public Kubernetes entry point is the ingress routing to the API gateway. Locally, the frontend can point to a configured gateway/API base URL. The Kubernetes manifest includes ingress TLS intent, but live cluster ingress evidence is still pending.

### 22. Why does Kubernetes not prove HA by itself?

Answer:

> Kubernetes can restart pods, but HA depends on correct replicas, probes, disruption budgets, node distribution, dependency availability, ingress behavior, image availability, and tested failover. A YAML file is only intent; live failure testing is evidence.

### 23. What observability do you have?

Answer:

> The services expose Prometheus-style metrics, propagate correlation IDs, generate/propagate W3C trace context, use structured logs, and include alert/dashboard config for async operations. Full OpenTelemetry span instrumentation and a verified trace backend are future work.

### 24. What would you improve next?

Answer:

> I would run heavy evidence in GitHub Actions, add real OpenTelemetry spans, deploy to a staging cluster with managed Postgres/RabbitMQ/Redis, add secret management and cert-manager TLS, implement outbox retention and replay tooling, and produce load-test reports with latency/throughput/error-rate thresholds.

### 25. If this were a production product, would you keep microservices?

Answer:

> Not necessarily. For a small team and early product, I would consider a modular monolith with the same internal boundaries. I would keep the outbox and idempotent async projections if fanout mattered, but I might deploy fewer processes until scale or team ownership justified service separation.

### 26. What tradeoff are you most proud of?

Answer:

> Fusing event and registration into one consistency boundary. It is less flashy than splitting every noun into a service, but it is the right design for correctness. It avoids distributed transactions for capacity and waitlist logic while still letting feed and notification work be asynchronous.

### 27. What is the biggest weakness of the project today?

Answer:

> The biggest weakness is evidence, not architecture. The design has many production-readiness pieces, but live HA, load, and deployment evidence are not complete. I would be careful to claim "Kubernetes-ready" and "designed for scalability" rather than "production-proven high availability."

### 28. How would you explain this to a non-technical interviewer?

Answer:

> I built an event app where people can publish and join local activities. The hard part is preventing overbooking and making sure background updates like feeds and notifications happen reliably even when parts of the system fail. I separated the immediate booking logic from background work, then added tests and operational checks around those failure cases.

### 29. What did you learn?

Answer:

> I learned that distributed systems are mostly about boundaries and failure modes, not just adding services. The more honest design is to say which operations are strongly consistent, which are eventually consistent, and exactly what evidence proves each claim.

### 30. What should your resume bullet say?

Answer:

> Built a React + TypeScript and Go microservices event platform with PostgreSQL transactional registration/waitlist logic, RabbitMQ-backed transactional outbox for asynchronous feed/notification projections, Redis caching/rate limiting, JWT/RBAC with rotating refresh tokens, and Kubernetes-ready manifests; verified with unit, integration, smoke, frontend, security, and static infrastructure checks.

### 31. Why React + TypeScript + Vite for the frontend?

Answer:

> The frontend had grown beyond static pages because it needed auth/session handling, role-aware actions, event publishing, joining/canceling, attendee moderation, admin role updates, runtime API configuration, and user feedback states. React gives a maintainable component model for those workflows, TypeScript gives compile-time checks around API payloads and state shape, and Vite gives fast local development plus a production bundle. The tradeoff is more tooling and dependencies, so I keep the frontend claim bounded as a workflow/product demo until accessibility, visual regression, and production hosting evidence are added.
