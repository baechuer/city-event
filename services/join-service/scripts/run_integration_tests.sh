#!/usr/bin/env bash
set -euo pipefail


PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.test.yml"
docker compose -f "$PROJECT_ROOT/docker-compose.test.yml" down -v || true
docker compose -f "$PROJECT_ROOT/docker-compose.test.yml" up -d
cleanup() {
  docker compose -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker compose -f "$COMPOSE_FILE" up -d

# Wait for Postgres
until docker exec -i join_service_test_db pg_isready -U test_user -d join_service_test >/dev/null 2>&1; do
  sleep 0.5
done

# Wait for Redis
until docker exec -i join_service_test_redis redis-cli ping >/dev/null 2>&1; do
  sleep 0.5
done

# Apply migrations
for f in "$PROJECT_ROOT"/migrations/*.sql; do
  docker exec -i join_service_test_db psql -U test_user -d join_service_test < "$f" >/dev/null
done

export TEST_DB_DSN="postgres://test_user:test_password@127.0.0.1:5433/join_service_test?sslmode=disable"
export TEST_REDIS_ADDR="127.0.0.1:6380"
export TEST_RABBIT_URL="amqp://guest:guest@localhost:5673/"
export TEST_RABBIT_EXCHANGE="city.events.test"

cd "$PROJECT_ROOT"
go test -tags=integration -count=1 -race ./...
