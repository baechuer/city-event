#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../" && pwd)"
SERVICE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Stand up infra
docker compose -f "$SERVICE_ROOT/test/integration/infra/compose.yaml" up -d

# Cleanup on exit
cleanup() {
  docker compose -f "$SERVICE_ROOT/test/integration/infra/compose.yaml" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Wait for infra (simplistic wait, or rely on go test helpers if they retry)
# Postgres: 5434
# Redis: 6381
# Rabbit: 5675

echo "Waiting for infra..."
sleep 5 # Giving docker a moment to map ports clearly (better to use logic like join-service script but this is quick)

# Export Env Vars for Test
export DATABASE_URL="postgres://user:pass@127.0.0.1:5434/app?sslmode=disable"
# Redis? Event service might use it? compose has it.
# Check infra/compose.yaml -> redis port 6381
export REDIS_ADDR="127.0.0.1:6381"
export RABBITMQ_URL="amqp://guest:guest@localhost:5675/"

# Run tests
cd "$SERVICE_ROOT"
go test -tags=integration ./test/integration/... -count=1 -p=1
