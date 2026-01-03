-- Track outbox for async event processing
CREATE TABLE track_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_id UUID NOT NULL,
    feed_type TEXT,
    position INT,
    request_id TEXT,
    bucket_date DATE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX ix_track_outbox_pending ON track_outbox(created_at) WHERE processed_at IS NULL;
