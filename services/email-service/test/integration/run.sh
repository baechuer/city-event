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

# Wait for infra
echo "Waiting for infra..."
sleep 5

# Export Env Vars for Test
# Redis: 6382
# Rabbit: 5676
# Mailpit: 8026 / 1026

export REDIS_ADDR="127.0.0.1:6382"
export RABBITMQ_URL="amqp://guest:guest@localhost:5676/"
export EMAIL_SENDER="smtp"
export SMTP_HOST="localhost"
export SMTP_PORT="1026"
export SMTP_USERNAME="user"
export SMTP_PASSWORD="password"
export SMTP_INSECURE="true"
export MAILPIT_API="http://localhost:8026"

# Run tests
cd "$SERVICE_ROOT"
go test -tags=integration ./test/integration/... -count=1 -p=1
