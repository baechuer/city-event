#!/bin/bash
set -euo pipefail

# Email Service Integration Test Config
# Matches the user's running docker infra
export RABBIT_URL="amqp://guest:guest@localhost:5672/"
export RABBIT_EXCHANGE="city.events"
export SMTP_HOST="127.0.0.1"
export SMTP_PORT="1025"
export REDIS_ADDR="localhost:6379"

echo "Running Email Service Integration Tests..."
cd services/email-service
go test -v -tags=integration -count=1 ./test/integration/...
