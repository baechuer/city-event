-- event_index: local copy of events for trending queries
-- Synced from event-service via batch pull or message subscription

CREATE TABLE event_index (
    event_id UUID PRIMARY KEY,
    title TEXT NOT NULL,
    city TEXT,
    tags TEXT[],
    start_time TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    owner_id TEXT,
    synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ix_event_index_city ON event_index(city);
CREATE INDEX ix_event_index_status_start ON event_index(status, start_time) WHERE status = 'published';
