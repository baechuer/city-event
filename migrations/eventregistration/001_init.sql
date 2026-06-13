CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    organizer_id TEXT NOT NULL,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    description TEXT NOT NULL DEFAULT '',
    city TEXT NOT NULL CHECK (length(trim(city)) > 0),
    venue TEXT NOT NULL CHECK (length(trim(venue)) > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    capacity INTEGER NOT NULL CHECK (capacity > 0),
    status TEXT NOT NULL CHECK (status IN ('PUBLISHED', 'CANCELED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS event_registrations (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('CONFIRMED', 'WAITLISTED', 'CANCELED')),
    waitlist_position INTEGER,
    idempotency_key TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_event_registrations_one_active
    ON event_registrations(event_id, user_id)
    WHERE status IN ('CONFIRMED', 'WAITLISTED');

CREATE INDEX IF NOT EXISTS idx_event_registrations_event_status
    ON event_registrations(event_id, status);

CREATE INDEX IF NOT EXISTS idx_event_registrations_waitlist
    ON event_registrations(event_id, waitlist_position, created_at)
    WHERE status = 'WAITLISTED';

CREATE TABLE IF NOT EXISTS outbox_messages (
    id TEXT PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SENT', 'FAILED', 'DEAD')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_messages_pending
    ON outbox_messages(status, available_at, created_at)
    WHERE status IN ('PENDING', 'FAILED');
