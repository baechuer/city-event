#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

skip_build=false
for arg in "$@"; do
  case "$arg" in
    --skip-build) skip_build=true ;;
    *) die "unknown argument: $arg" ;;
  esac
done

cd "$REPO_ROOT"
setup_go_cache

postgres_url="postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
rabbit_url="amqp://cityevents:cityevents@localhost:5672/"
export PHASE4_TEST_DATABASE_URL="${PHASE4_TEST_DATABASE_URL:-$postgres_url}"
export PHASE4_TEST_RABBITMQ_URL="${PHASE4_TEST_RABBITMQ_URL:-$rabbit_url}"

log "Default Go tests"
run_go test ./...

log "Ensure Postgres and RabbitMQ are running"
run_docker compose up -d postgres rabbitmq
wait_for_compose_health postgres 120
wait_for_compose_health rabbitmq 120

log "Integration tests"
run_go test -count=1 -tags=integration \
  ./internal/platform/messaging \
  ./internal/services/eventregistration \
  ./internal/services/feedprojection \
  ./internal/services/outboxrelay

if [[ "$skip_build" == true ]]; then
  echo "Worker build skipped."
  exit 0
fi

log "Worker builds"
mkdir -p tmp
run_go build -o tmp/outbox-relay.exe ./cmd/outbox-relay
run_go build -o tmp/feed-worker.exe ./cmd/feed-worker

echo "Phase 4 verification completed."
