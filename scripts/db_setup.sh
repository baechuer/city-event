#!/bin/bash
set -e

echo "Waiting for postgres to be ready..."
sleep 5 # Simple wait, or use pg_isready if available locally

# Helper function to run SQL
apply_sql() {
    service=$1
    db_name=$2
    file=$3
    echo "Applying $file to $db_name (Service: $service)..."
    cat "$file" | docker exec -i cityevents-postgres psql -U postgres -d "$db_name"
}

# Auth Service
echo ">>> Migrating Auth Service"
apply_sql "auth" "auth_db" "services/auth-service/migrations/001_init.sql"

# Event Service
echo ">>> Migrating Event Service"
for file in services/event-service/migrations/*.sql; do
    apply_sql "event" "event_db" "$file"
done

# Join Service
echo ">>> Migrating Join Service"
for file in services/join-service/migrations/*.sql; do
    apply_sql "join" "join_db" "$file"
done

echo "✅ All migrations applied successfully!"
