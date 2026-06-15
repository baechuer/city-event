#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

skip_runtime_smoke=false
for arg in "$@"; do
  case "$arg" in
    --skip-runtime-smoke) skip_runtime_smoke=true ;;
    *) die "unknown argument: $arg" ;;
  esac
done

cd "$REPO_ROOT"
setup_go_cache

postgres_url="postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
rabbit_url="amqp://cityevents:cityevents@localhost:5672/"
redis_url="redis://localhost:6379/0"
export PHASE5_TEST_DATABASE_URL="${PHASE5_TEST_DATABASE_URL:-$postgres_url}"
export PHASE5_TEST_REDIS_URL="${PHASE5_TEST_REDIS_URL:-$redis_url}"
export PHASE4_TEST_DATABASE_URL="${PHASE4_TEST_DATABASE_URL:-$postgres_url}"
export PHASE4_TEST_RABBITMQ_URL="${PHASE4_TEST_RABBITMQ_URL:-$rabbit_url}"

log "Default Go tests"
run_go test ./...

log "Ensure Postgres, RabbitMQ, and Redis are running"
run_docker compose up -d postgres rabbitmq redis
wait_for_compose_health postgres 120
wait_for_compose_health rabbitmq 120
wait_for_compose_health redis 120

log "Integration tests"
run_go test -count=1 -p 1 -tags=integration \
  ./internal/services/eventregistration \
  ./internal/services/feed \
  ./internal/services/feedprojection \
  ./internal/services/outboxrelay

if [[ "$skip_runtime_smoke" == true ]]; then
  echo "Runtime smoke skipped."
  exit 0
fi

log "Feed service runtime smoke"
mkdir -p tmp
run_go build -o tmp/feed-service.exe ./cmd/feed-service

cleanup_smoke() {
  cleanup_pid "${pid:-}"
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "\$target = (Resolve-Path 'tmp/feed-service.exe').Path; Get-CimInstance Win32_Process -Filter \"Name = 'feed-service.exe'\" | Where-Object { \$_.ExecutablePath -eq \$target } | ForEach-Object { Stop-Process -Id \$_.ProcessId -Force }" >/dev/null 2>&1 || true
  fi
}

port=18085
if command -v cmd.exe >/dev/null 2>&1; then
  smoke_binary="$(windows_binary_path "$REPO_ROOT/tmp/feed-service.exe")"
  cmd.exe /d /c "set FEED_SERVICE_HTTP_ADDR=127.0.0.1:$port && set POSTGRES_URL=$postgres_url && set REDIS_URL=$redis_url && $smoke_binary" >tmp/feed-service-smoke.log 2>&1 &
else
  FEED_SERVICE_HTTP_ADDR="127.0.0.1:$port" \
  POSTGRES_URL="$postgres_url" \
  REDIS_URL="$redis_url" \
    ./tmp/feed-service.exe >tmp/feed-service-smoke.log 2>&1 &
fi
pid=$!
trap cleanup_smoke EXIT

if ! wait_for_http "http://127.0.0.1:$port/readyz" 20; then
  cat tmp/feed-service-smoke.log || true
  die "feed-service did not become ready."
fi

list="$(http_curl -fsS "http://127.0.0.1:$port/v1/feed/events?limit=5")"
grep -Fq '"events"' <<<"$list" || die "feed list response did not contain events array."

cleanup_smoke
trap - EXIT

echo "Phase 5 verification completed."
