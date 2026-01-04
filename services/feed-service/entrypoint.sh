#!/bin/sh
set -e

# Wait for Postgres
echo "Waiting for PostgreSQL..."
until nc -z ${DB_HOST:-cityevents-postgres} ${DB_PORT:-5432}; do
  echo "Database is unavailable - sleeping"
  sleep 2
done
echo "Database is up."

# Run migrations
echo "Running database migrations..."
migrate -path=/app/migrations -database "${DB_ADDR}" up

# Start application
echo "Starting feed-service..."
exec ./feed-service
