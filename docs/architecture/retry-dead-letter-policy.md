# Retry And Dead-Letter Policy

## Purpose

CityEvents uses RabbitMQ for at-least-once delivery. Failed asynchronous work must not loop forever in the live queue.

The implemented policy is:

- retry transient consumer failures with increasing delay
- move exhausted consumer messages to a DLQ
- retry failed outbox publishes with increasing `available_at`
- mark exhausted outbox rows `DEAD`
- keep resume wording at "at-least-once delivery with idempotent business effects"

## RabbitMQ Consumer Retry

Feed projection and notification delivery use the same bounded policy.

| Attempt | Delay Before Redelivery |
| --- | --- |
| 1 | 5 seconds |
| 2 | 30 seconds |
| 3 | 2 minutes |
| 4 | 10 minutes |
| 5 | 30 minutes |

Implementation:

- `internal/platform/messaging/rabbitmq.go` declares `cityevents.retry` and `cityevents.retry.return`.
- Each consumer queue has per-attempt retry queues with `x-message-ttl`.
- A failed message is copied to the next retry queue with RabbitMQ publisher confirms.
- The original delivery is acked only after the retry/DLQ copy is confirmed.
- After `MaxConsumerRetries`, the message is published to `cityevents.dlx` and lands in the service DLQ.
- If retry/DLQ publishing fails, the original delivery is requeued to avoid dropping it.

Relevant files:

- `internal/platform/messaging/envelope.go`
- `internal/platform/messaging/rabbitmq.go`
- `internal/services/feedprojection/consumer.go`
- `internal/services/notification/consumer.go`

## Outbox Publish Retry

The event-registration outbox is the durable producer-side buffer.

| Recorded Failure Count | Next Retry Delay |
| --- | --- |
| 1 | 1 second |
| 2 | 4 seconds |
| 3 | 15 seconds |
| 4 | 1 minute |
| 5 | 5 minutes |
| 6 | 15 minutes |
| 7 | 1 hour |
| 8 | terminal `DEAD` |

Implementation:

- `PENDING` and retryable `FAILED` rows are selected with `FOR UPDATE SKIP LOCKED`.
- A row becomes `SENT` only after RabbitMQ confirms the publish.
- A publish or envelope error increments `attempts`.
- Attempts below the limit become `FAILED` with a future `available_at`.
- Attempt 8 becomes `DEAD` and is no longer selected by the relay.

Relevant files:

- `internal/services/outboxrelay/relay.go`
- `migrations/eventregistration/001_init.sql`
- `migrations/eventregistration/002_outbox_relay.sql`
- `migrations/eventregistration/003_outbox_dead_state.sql`

## Verification

Safe local checks:

```bash
go test ./internal/platform/messaging ./internal/services/feedprojection ./internal/services/notification ./internal/services/outboxrelay
```

Covered by unit tests:

- consumer retry delays are strictly increasing
- retry-count headers parse safely
- original routing key is preserved after delayed retry redelivery
- outbox retry delays are strictly increasing
- outbox terminal state starts at `MaxOutboxPublishAttempts`

Covered by integration tests when RabbitMQ is available:

- `TestPublishFailedDeliveryRoutesRetryAndDLQ` verifies failed messages are routed to the first retry queue and exhausted messages land in the feed DLQ.
- `TestRelayMarksOutboxDeadAfterAttemptLimit` verifies exhausted outbox publish failures become terminal `DEAD` rows.

Integration and failure evidence still belongs in isolated runners:

- RabbitMQ topology and publish smoke: integration tests / phase verification
- broker outage and recovery: manual GitHub Actions Heavy Evidence workflow
- Kubernetes failure smoke: manual GitHub Actions Heavy Evidence workflow

Operator tools:

- `scripts/inspect-async-ops.sh` writes read-only outbox, retry queue, and DLQ evidence.
- `scripts/copy-rabbitmq-dlq.sh` copies DLQ messages back for reprocessing after root-cause remediation.
- `scripts/requeue-dead-outbox.sh` requeues selected `DEAD` outbox rows after root-cause remediation.
- `docs/operations/async-failure-runbook.md` records the inspect-first recovery flow.

## Caveats

- This is not exactly-once RabbitMQ consumption.
- A confirmed retry publish followed by an ack failure can still create a duplicate; idempotent consumers are required.
- DLQs prevent endless hot-loop retries, but they do not automatically repair bad messages.
- Operators still need alerting, dashboards, and reviewed replay evidence before production.
- Production RabbitMQ should use HA queues or a managed broker; this repo currently documents and tests application-level retry behavior, not broker high availability.
