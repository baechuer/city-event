#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

output_dir=""

usage() {
  cat <<'EOF'
Usage: ./scripts/inspect-async-ops.sh [options]

Writes read-only async operations evidence for outbox, retry queues, and DLQs.
This script does not start, stop, purge, replay, or mutate any dependency.

Options:
  --output-dir DIR  Directory for summary.md and raw command output.
                    Default: tmp/async-ops-inspection/<timestamp>
  -h, --help        Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir)
      [[ $# -ge 2 ]] || die "--output-dir requires a value"
      output_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

cd "$REPO_ROOT"

if [[ -z "$output_dir" ]]; then
  output_dir="$REPO_ROOT/tmp/async-ops-inspection/$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
elif [[ "$output_dir" != /* ]]; then
  output_dir="$REPO_ROOT/$output_dir"
fi
mkdir -p "$output_dir"

summary_file="$output_dir/summary.md"
postgres_file="$output_dir/postgres-outbox.md"
rabbitmq_file="$output_dir/rabbitmq-queues.md"
compose_file="$output_dir/compose-ps.txt"

postgres_query() {
  local title="$1"
  local sql="$2"
  {
    echo "## $title"
    echo
    run_docker compose exec -T postgres \
      psql -U cityevents -d cityevents -v ON_ERROR_STOP=1 -c "$sql"
    echo
  } >>"$postgres_file" 2>&1 || true
}

: >"$postgres_file"
: >"$rabbitmq_file"

{
  echo "# Async Operations Inspection"
  echo
  echo "- Captured At UTC: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "- Output Directory: $output_dir"
  echo "- Mode: read-only"
  echo
  echo "## Validation Purpose"
  echo
  echo "This report helps explain whether async failures are in producer outbox state, RabbitMQ queue backlog, retry queues, DLQs, or downstream consumers."
  echo
  echo "It does not prove high availability and does not replay messages."
  echo
} >"$summary_file"

if ! command -v docker >/dev/null 2>&1; then
  {
    echo
    echo "## Docker"
    echo
    echo "Docker is not available; Compose-backed inspection skipped."
  } >>"$summary_file"
  echo "Async operations inspection written to $summary_file"
  exit 0
fi

run_docker compose ps >"$compose_file" 2>&1 || true

postgres_cid="$(run_docker compose ps -q postgres 2>/dev/null || true)"
if [[ -n "$postgres_cid" ]]; then
  postgres_query "Outbox Status Counts" \
    "select status, count(*) from outbox_messages group by status order by status;"
  postgres_query "Retryable Outbox Rows" \
    "select status, count(*) from outbox_messages where status in ('PENDING','FAILED') group by status order by status;"
  postgres_query "Oldest Retryable Outbox Rows" \
    "select id, aggregate_type, aggregate_id, routing_key, status, attempts, available_at, left(last_error, 160) as last_error from outbox_messages where status in ('PENDING','FAILED') order by available_at asc, created_at asc limit 20;"
  postgres_query "Dead Outbox Rows" \
    "select id, aggregate_type, aggregate_id, routing_key, attempts, created_at, left(last_error, 160) as last_error from outbox_messages where status = 'DEAD' order by created_at desc limit 20;"
else
  {
    echo "## Postgres"
    echo
    echo "Postgres Compose service is not running."
  } >>"$postgres_file"
fi

rabbitmq_cid="$(run_docker compose ps -q rabbitmq 2>/dev/null || true)"
if [[ -n "$rabbitmq_cid" ]]; then
  {
    echo "# RabbitMQ Queue Snapshot"
    echo
    run_docker compose exec -T rabbitmq \
      rabbitmqctl list_queues name messages messages_ready messages_unacknowledged consumers arguments
  } >"$rabbitmq_file" 2>&1 || true
else
  {
    echo "# RabbitMQ Queue Snapshot"
    echo
    echo "RabbitMQ Compose service is not running."
  } >"$rabbitmq_file"
fi

{
  echo
  echo "## Evidence Files"
  echo
  echo "- Compose: $compose_file"
  echo "- Postgres outbox: $postgres_file"
  echo "- RabbitMQ queues: $rabbitmq_file"
  echo
  echo "## How To Read"
  echo
  echo "- Non-zero DLQ queue depth means a consumer exhausted its retry budget."
  echo "- Non-zero retry queue depth means delayed retry is active, not necessarily broken."
  echo "- Non-zero outbox \`DEAD\` count means producer-side publish/envelope failures exhausted their retry budget."
  echo "- Growing \`PENDING\` or due \`FAILED\` outbox rows point at relay or broker availability."
  echo "- Growing unacknowledged RabbitMQ messages point at slow or stuck consumers."
} >>"$summary_file"

echo "Async operations inspection written to $summary_file"
