#!/bin/sh
set -e

echo "Waiting for database to be ready..."
while ! pg_isready -h cityevents-postgres -p 5432 -U postgres > /dev/null 2>&1; do
  sleep 1
done
echo "Database is up."

echo "Running database migrations..."
migrate -path ./migrations -database "${DATABASE_URL}" up

echo "Starting media-service..."
exec ./server
