#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    CREATE DATABASE event_db;
    CREATE DATABASE join_db;
    CREATE DATABASE feed_db;
    CREATE DATABASE media_db;
EOSQL

