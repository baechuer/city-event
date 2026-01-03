#!/bin/sh
set -e

echo "Waiting for database to be ready..."

# Wait for database to be ready (connection check) using pg_isready if available
until pg_isready -h cityevents-postgres -p 5432 -U "${POSTGRES_USER:-postgres}"; do
  echo "Database is unavailable - sleeping"
  sleep 2
done

echo "Database is up."

# Determine migration connection string
MIGRATIONS_QUERY=""
if [ -n "$MIGRATIONS_TABLE" ]; then
  MIGRATIONS_QUERY="&x-migrations-table=$MIGRATIONS_TABLE"
fi
DB_URL="${DB_ADDR}${MIGRATIONS_QUERY}"

# Check current version and handle 'dirty' state in development
echo "Checking migration status..."
VERSION_OUTPUT=$(migrate -path=/app/migrations -database="$DB_URL" version 2>&1 || true)
echo "$VERSION_OUTPUT"

if echo "$VERSION_OUTPUT" | grep -q "dirty"; then
    if [ "$APP_ENV" = "dev" ] || [ "$APP_ENV" = "development" ]; then
        echo "Detected dirty migration state in development. Forcing clean state..."
        # Extract version number (e.g., from "1 (dirty)")
        CURRENT_VERSION=$(echo "$VERSION_OUTPUT" | head -n 1 | awk '{print $1}')
        if [ -n "$CURRENT_VERSION" ]; then
            migrate -path=/app/migrations -database="$DB_URL" force "$CURRENT_VERSION"
            echo "Forced version to $CURRENT_VERSION"
        else
            echo "Could not parse version, skipping force."
        fi
    else
        echo "CRITICAL: Database is in a dirty state. Manual intervention required."
        exit 1
    fi
fi

# Run migrations
echo "Running database migrations..."
migrate -path=/app/migrations -database="$DB_URL" up

echo "Migrations completed successfully"

# Start the application
exec ./server
