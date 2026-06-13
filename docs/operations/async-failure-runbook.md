# Async Failure Runbook

## Purpose

This runbook covers RabbitMQ retry queues, DLQs, and terminal outbox `DEAD`
rows. It is for operator recovery and debugging evidence, not for claiming
exactly-once consumption or production high availability.

## First Response

1. Inspect state before changing anything:

   ```bash
   ./scripts/inspect-async-ops.sh
   ```

2. Check the generated `summary.md`, Postgres outbox report, and RabbitMQ queue
   report.

3. Classify the failure:

   | Evidence | Likely Meaning |
   | --- | --- |
   | Retry queues have messages | delayed retry is active |
   | DLQ has messages | consumer retry budget was exhausted |
   | `outbox_messages.status = 'DEAD'` | producer-side publish or envelope retry budget was exhausted |
   | due `FAILED` outbox rows grow | relay cannot publish or broker is unavailable |
   | queue `messages_unacknowledged` grows | consumer is slow, stuck, or failing mid-processing |

4. Fix the root cause before replay:

   - bad payload or unsupported routing key: fix producer/consumer code
   - DB outage: restore DB and verify readiness
   - RabbitMQ outage: restore broker and verify queues
   - schema mismatch: apply migration safely
   - poison message: decide whether to discard, patch, or replay

## RabbitMQ DLQ Copy-Back

Dry-run:

```bash
./scripts/copy-rabbitmq-dlq.sh --queue cityevents.feed.projection.dlq --count 10
```

Apply in GitHub Actions, or locally only with an explicit guard:

```bash
CITYEVENTS_ALLOW_LOCAL_REPLAY=true \
  ./scripts/copy-rabbitmq-dlq.sh --queue cityevents.feed.projection.dlq --count 10 --apply
```

The script copies messages back to `cityevents.events` using the original
routing key and strips retry headers. It reads with `ack_requeue_true`, so the
DLQ copy remains in place. Purging the DLQ is a separate manual decision after
successful replay is verified.

This script uses the RabbitMQ management HTTP API, so the management plugin or
equivalent endpoint must be available.

## Outbox DEAD Requeue

Dry-run:

```bash
./scripts/requeue-dead-outbox.sh --limit 10
```

Apply one row:

```bash
CITYEVENTS_ALLOW_LOCAL_REPLAY=true \
  ./scripts/requeue-dead-outbox.sh --id <outbox-id> --apply
```

Reset attempts only when the cause is known to be fixed and a fresh retry budget
is intentional:

```bash
CITYEVENTS_ALLOW_LOCAL_REPLAY=true \
  ./scripts/requeue-dead-outbox.sh --id <outbox-id> --reset-attempts --apply
```

## Validation After Replay

Run inspection again:

```bash
./scripts/inspect-async-ops.sh
```

Expected signs:

- DLQ depth does not continue growing
- retry queues drain after their TTLs
- no new `DEAD` outbox rows for the same routing key
- feed projection or notification record appears where expected
- `/metrics` shows retry/dead-letter counters for the recovery path

## Claim Boundary

Safe wording:

```text
Added operator runbooks and guarded tooling for inspecting async backlog, copying RabbitMQ DLQ messages for reprocessing, and requeueing terminal outbox rows after root-cause remediation.
```

Not safe:

```text
Automated production-grade DLQ recovery.
```
