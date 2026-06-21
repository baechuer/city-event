CREATE TABLE IF NOT EXISTS event_audit_events (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_id TEXT NOT NULL,
    target_type TEXT NOT NULL,
    result TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_event_audit_events_actor_created
    ON event_audit_events(actor_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_event_audit_events_target_created
    ON event_audit_events(target_id, created_at DESC);
