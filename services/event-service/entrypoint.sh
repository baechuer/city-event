#!/bin/sh
set -e

echo "Running database migrations..."

# Wait for database to be ready
until migrate -path=/app/migrations -database="$DATABASE_URL" up 2>&1; do
  echo "Migration failed, retrying in 2 seconds..."
  sleep 2
done

echo "Migrations completed successfully"

# Start the application
exec ./server
