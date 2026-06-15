#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

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

log "Integration tests"
run_go test -count=1 -p 1 -tags=integration ./...

log "Debug walkthrough check"
walkthrough="project-center/20-audits/debugging-walkthrough.md"
require_file "$walkthrough"
for term in X-Correlation-ID outbox_messages feed_events notifications; do
  require_contains "$walkthrough" "$term"
done

echo "Phase 9 verification completed."
