# cleanup on exit (even if fail)
cleanup() {
  docker compose -f test/integration/infra/compose.yaml down -v
}
trap cleanup EXIT

docker compose -f test/integration/infra/compose.yaml up -d

# optional: show status
docker compose -f test/integration/infra/compose.yaml ps

# Export ephemeral ports from infra/compose.yaml
# Postgres: 54321
# Redis: 63791
# RabbitMQ: 56731 / 15671

export POSTGRES_DSN="postgres://user:pass@127.0.0.1:54321/app?sslmode=disable"
export REDIS_ADDR="127.0.0.1:63791"
export RABBIT_URL="amqp://it:it@127.0.0.1:56731/"

go test -tags=integration ./test/integration/... -count=1 -p=1

docker compose -f test/integration/infra/compose.yaml down -v
