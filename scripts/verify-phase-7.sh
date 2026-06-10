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
smtp_addr="localhost:1025"
minio_endpoint="http://localhost:9000"
minio_access_key="cityevents"
minio_secret_key="cityevents-password"
minio_bucket="cityevents-media-test"
export PHASE4_TEST_DATABASE_URL="${PHASE4_TEST_DATABASE_URL:-$postgres_url}"
export PHASE4_TEST_RABBITMQ_URL="${PHASE4_TEST_RABBITMQ_URL:-$rabbit_url}"
export PHASE5_TEST_DATABASE_URL="${PHASE5_TEST_DATABASE_URL:-$postgres_url}"
export PHASE5_TEST_REDIS_URL="${PHASE5_TEST_REDIS_URL:-$redis_url}"
export PHASE6_TEST_DATABASE_URL="${PHASE6_TEST_DATABASE_URL:-$postgres_url}"
export PHASE6_TEST_RABBITMQ_URL="${PHASE6_TEST_RABBITMQ_URL:-$rabbit_url}"
export PHASE6_TEST_SMTP_ADDR="${PHASE6_TEST_SMTP_ADDR:-$smtp_addr}"
export PHASE7_TEST_DATABASE_URL="${PHASE7_TEST_DATABASE_URL:-$postgres_url}"
export PHASE7_TEST_MINIO_ENDPOINT="${PHASE7_TEST_MINIO_ENDPOINT:-$minio_endpoint}"
export PHASE7_TEST_MINIO_ACCESS_KEY="${PHASE7_TEST_MINIO_ACCESS_KEY:-$minio_access_key}"
export PHASE7_TEST_MINIO_SECRET_KEY="${PHASE7_TEST_MINIO_SECRET_KEY:-$minio_secret_key}"
export PHASE7_TEST_MINIO_BUCKET="${PHASE7_TEST_MINIO_BUCKET:-$minio_bucket}"

log "Default Go tests"
run_go test ./...

log "Ensure local dependencies are running"
run_docker compose up -d postgres rabbitmq redis minio mailpit
wait_for_compose_health postgres 120
wait_for_compose_health rabbitmq 120
wait_for_compose_health redis 120
wait_for_compose_health minio 120
wait_for_tcp 127.0.0.1 1025 60

log "Integration tests"
run_go test -count=1 -tags=integration ./...

log "Media service and worker builds"
mkdir -p tmp
run_go build -o tmp/media-service.exe ./cmd/media-service
run_go build -o tmp/media-worker.exe ./cmd/media-worker

if [[ "$skip_runtime_smoke" == true ]]; then
  echo "Runtime smoke skipped."
  exit 0
fi

log "Media service runtime smoke"
port=18087
MEDIA_SERVICE_HTTP_ADDR="127.0.0.1:$port" \
POSTGRES_URL="$postgres_url" \
MINIO_ENDPOINT="$minio_endpoint" \
MINIO_ACCESS_KEY="$minio_access_key" \
MINIO_SECRET_KEY="$minio_secret_key" \
MINIO_BUCKET="$minio_bucket" \
  ./tmp/media-service.exe >tmp/media-service-smoke.log 2>&1 &
pid=$!
trap 'cleanup_pid "$pid"' EXIT

if ! wait_for_http "http://127.0.0.1:$port/readyz" 20; then
  cat tmp/media-service-smoke.log || true
  die "media-service did not become ready."
fi

body='{"eventId":"phase7-event","filename":"banner.jpg","contentType":"image/jpeg","sizeBytes":10}'
created="$(curl -fsS -X POST "http://127.0.0.1:$port/v1/media/uploads" -H "Content-Type: application/json" -H "X-User-ID: phase7-user" --data "$body")"
grep -Fq '"uploadUrl"' <<<"$created" || die "create upload did not return upload URL."
asset_id="$(json_string_field "$created" id)"
[[ -n "$asset_id" ]] || die "create upload did not return asset id."
detail="$(curl -fsS "http://127.0.0.1:$port/v1/media/$asset_id")"
grep -Fq '"status":"UPLOADING"' <<<"$detail" || die "media detail did not return UPLOADING."

cleanup_pid "$pid"
trap - EXIT

echo "Phase 7 verification completed."
