#!/bin/bash
set -euo pipefail

# Tests expect IT_ prefix for integration config
export IT_PG_DSN="postgres://user:pass@127.0.0.1:5432/app?sslmode=disable"
export IT_REDIS_ADDR="localhost:6379"
export IT_RABBIT_URL="amqp://guest:guest@localhost:5672/"

echo "Running Auth Service Integration Tests..."
echo "Config:"
echo "  IT_PG_DSN: $IT_PG_DSN"
echo "  IT_REDIS_ADDR: $IT_REDIS_ADDR"
echo "  IT_RABBIT_URL: $IT_RABBIT_URL"

cd services/auth-service
go test -v -tags=integration -count=1 ./test/integration/cases/...
