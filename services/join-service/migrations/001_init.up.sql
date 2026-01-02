-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Event Capacity (Snapshots/Cache)
-- This table stores a local copy of event constraints.
-- When event-service publishes an event, we store its capacity here.
-- capacity = 0 represents unlimited capacity.
CREATE TABLE IF NOT EXISTS event_capacity (
    event_id UUID PRIMARY KEY,
    capacity INTEGER NOT NULL DEFAULT 0, 
    active_count INTEGER NOT NULL DEFAULT 0,
    waitlist_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Joins (Registration records)
-- Tracks the state of a user's participation in an event.
CREATE TYPE join_status AS ENUM ('active', 'waitlisted', 'canceled', 'expired', 'rejected');

CREATE TABLE IF NOT EXISTS joins (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id UUID NOT NULL, -- FK-like reference to event_capacity
    user_id UUID NOT NULL,  -- Extracted from JWT
    status join_status NOT NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    canceled_at TIMESTAMPTZ,
    activated_at TIMESTAMPTZ, -- Timestamp when status moved from 'waitlisted' to 'active'
    
    -- Constraint: A user can only have one registration per event
    CONSTRAINT uq_event_user UNIQUE (event_id, user_id)
);

-- Index: FIFO Queue lookup for waitlist processing 
-- Used to find the earliest 'waitlisted' participant when a spot opens up.
CREATE INDEX IF NOT EXISTS idx_joins_waitlist_fifo ON joins (event_id, created_at ASC) WHERE status = 'waitlisted';

-- Index: User history lookup
CREATE INDEX IF NOT EXISTS idx_joins_user_history ON joins (user_id, created_at DESC);

-- 3. Outbox (Transactional Outbox Pattern)
-- Ensures atomicity between database updates and message publishing.
CREATE TYPE outbox_status AS ENUM ('pending', 'sent', 'failed');

CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    trace_id TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status outbox_status NOT NULL DEFAULT 'pending',
    
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    next_retry_at TIMESTAMPTZ
);

-- Index: Outbox worker polling
-- Optimizes selection of pending messages for the relay worker.
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox (next_retry_at) WHERE status = 'pending';

-- 4. Processed Messages (Idempotency/Deduplication)
-- Tracks incoming message IDs to prevent double-processing (at-least-once delivery).
CREATE TABLE IF NOT EXISTS processed_messages (
    message_id TEXT PRIMARY KEY,
    handler_name TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);