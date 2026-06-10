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
redis_url="redis://localhost:6379/0"
smtp_addr="localhost:1025"
export PHASE4_TEST_DATABASE_URL="${PHASE4_TEST_DATABASE_URL:-$postgres_url}"
export PHASE4_TEST_RABBITMQ_URL="${PHASE4_TEST_RABBITMQ_URL:-$rabbit_url}"
export PHASE5_TEST_DATABASE_URL="${PHASE5_TEST_DATABASE_URL:-$postgres_url}"
export PHASE5_TEST_REDIS_URL="${PHASE5_TEST_REDIS_URL:-$redis_url}"
export PHASE6_TEST_DATABASE_URL="${PHASE6_TEST_DATABASE_URL:-$postgres_url}"
export PHASE6_TEST_RABBITMQ_URL="${PHASE6_TEST_RABBITMQ_URL:-$rabbit_url}"
export PHASE6_TEST_SMTP_ADDR="${PHASE6_TEST_SMTP_ADDR:-$smtp_addr}"

log "Default Go tests"
run_go test ./...

log "Ensure Postgres, RabbitMQ, Redis, and Mailpit are running"
run_docker compose up -d postgres rabbitmq redis mailpit
wait_for_compose_health postgres 120
wait_for_compose_health rabbitmq 120
wait_for_compose_health redis 120
wait_for_tcp 127.0.0.1 1025 60

log "Integration tests"
run_go test -count=1 -tags=integration ./...

if [[ "$skip_build" == true ]]; then
  echo "Worker build skipped."
  exit 0
fi

log "Notification worker build"
mkdir -p tmp
run_go build -o tmp/notification-worker.exe ./cmd/notification-worker
run_go build -o tmp/notification-service.exe ./cmd/notification-service

echo "Phase 6 verification completed."
