CREATE TABLE IF NOT EXISTS feed_events (
    event_id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    city TEXT NOT NULL DEFAULT '',
    venue TEXT NOT NULL DEFAULT '',
    starts_at TIMESTAMPTZ,
    capacity INTEGER NOT NULL DEFAULT 0 CHECK (capacity >= 0),
    status TEXT NOT NULL DEFAULT 'PUBLISHED',
    confirmed_count INTEGER NOT NULL DEFAULT 0 CHECK (confirmed_count >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS processed_messages (
    consumer_name TEXT NOT NULL,
    message_id TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_name, message_id)
);
