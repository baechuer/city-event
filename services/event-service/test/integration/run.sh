#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../" && pwd)"
SERVICE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Stand up infra
docker compose -f "$SERVICE_ROOT/test/integration/infra/compose.yaml" up -d

# Cleanup on exit
cleanup() {
  docker compose -f "$SERVICE_ROOT/test/integration/infra/compose.yaml" down -v >/dev/null 2>&1 || true
  # Cat app log if exists
  if [ -f app.log ]; then
     echo "=== APP LOGS ==="
     cat app.log
     rm app.log
  fi
}
trap cleanup EXIT

# Wait for infra (simplistic wait, or rely on go test helpers if they retry)
# Postgres: 5434
# Redis: 6381
# Rabbit: 5675

echo "Waiting for infra..."
sleep 5 # Giving docker a moment to map ports clearly (better to use logic like join-service script but this is quick)

# Export Env Vars for Test
export HTTP_ADDR=":8081"
export EVENT_BASE_URL="http://localhost:8081"
export DATABASE_URL="postgres://user:pass@127.0.0.1:5434/app?sslmode=disable"
# Redis? Event service might use it? compose has it.
# Check infra/compose.yaml -> redis port 6381
export REDIS_URL="redis://127.0.0.1:6381/0"
export RABBIT_URL="amqp://guest:guest@localhost:5675/"

# Apply Migrations
echo "Applying migrations..."
for file in "$SERVICE_ROOT/migrations"/*.sql; do
  echo "Applying $file..."
  docker exec -i cityevents-event-it-postgres psql -U user -d app < "$file"
done

# Start App in background
echo "Building app..."
go build -o app api/cmd/main.go

echo "Starting app..."
# export vars needed by app (already exported above? check)
# DATABASE_URL is set.
# HTTP_ADDR is set.
# REDIS_ADDR is set.
# RABBIT_URL is set.
./app > app.log 2>&1 &
APP_PID=$!

# Wait for healthy
echo "Waiting for healthz..."
# primitive wait
timeout=30
while ! curl -s http://localhost:8081/healthz >/dev/null; do
  sleep 1
  timeout=$((timeout-1))
  if [ "$timeout" -le 0 ]; then
    echo "Timeout waiting for healthz"
    cat app.log
    kill $APP_PID
    exit 1
  fi
done

echo "App ready!"

# Run tests
cd "$SERVICE_ROOT"
# go test command...
go test -tags=integration ./test/integration/... -count=1 -p=1

kill $APP_PID
