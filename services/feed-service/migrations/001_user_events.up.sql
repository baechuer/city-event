-- feed-service: user_events table (partitioned by bucket_date)
-- Stores impression/view/join events for trending + personalization

CREATE TABLE user_events (
    id UUID DEFAULT gen_random_uuid(),
    actor_key TEXT NOT NULL,  -- "u:<user_id>" or "a:<anon_id>"
    event_type TEXT NOT NULL CHECK (event_type IN ('impression', 'view', 'join')),
    event_id UUID NOT NULL,
    feed_type TEXT,           -- 'trending', 'personalized', 'latest'
    position INT,             -- position in feed (for impression)
    request_id TEXT,          -- for tracing
    occurred_at TIMESTAMPTZ DEFAULT NOW(),
    bucket_date DATE NOT NULL, -- (occurred_at AT TIME ZONE 'UTC')::date
    PRIMARY KEY (id, bucket_date)
) PARTITION BY RANGE (bucket_date);

-- Create initial partitions (2026)
CREATE TABLE user_events_2026_01 PARTITION OF user_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE user_events_2026_02 PARTITION OF user_events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE user_events_2026_03 PARTITION OF user_events
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- Dedup index: includes bucket_date (partition key) for valid cross-partition uniqueness
CREATE UNIQUE INDEX ux_user_events_dedup 
    ON user_events(actor_key, event_id, event_type, bucket_date);

-- Query indexes
CREATE INDEX ix_user_events_event ON user_events(event_id, bucket_date DESC);
CREATE INDEX ix_user_events_actor ON user_events(actor_key, bucket_date DESC);
