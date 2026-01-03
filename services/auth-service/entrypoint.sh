#!/bin/sh
set -e

echo "Running database migrations..."

# Wait for database to be ready
MIGRATIONS_QUERY=""
if [ -n "$MIGRATIONS_TABLE" ]; then
  MIGRATIONS_QUERY="&x-migrations-table=$MIGRATIONS_TABLE"
fi

until migrate -path=/app/migrations -database="${DB_ADDR}${MIGRATIONS_QUERY}" up 2>&1; do
  echo "Migration failed, retrying in 2 seconds..."
  sleep 2
done

echo "Migrations completed successfully"

# Start the application
exec ./server
