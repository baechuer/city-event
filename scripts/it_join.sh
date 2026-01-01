#!/bin/bash
set -euo pipefail

# Join Service Integration Test Config
# Matches the user's running docker infra
export TEST_DB_DSN="postgres://user:pass@127.0.0.1:5432/app?sslmode=disable"
export TEST_REDIS_ADDR="127.0.0.1:6379"
export TEST_RABBIT_URL="amqp://guest:guest@localhost:5672/"
export TEST_RABBIT_EXCHANGE="city.events"

echo "Running Join Service Integration Tests..."
cd services/join-service
go test -v -tags=integration -count=1 ./internal/infrastructure/postgres/... ./internal/infrastructure/redis/...
