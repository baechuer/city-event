CREATE TABLE IF NOT EXISTS auth_audit_events (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_user_id TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT 'user',
    result TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_auth_audit_events_actor_created
    ON auth_audit_events(actor_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_auth_audit_events_target_created
    ON auth_audit_events(target_user_id, created_at DESC);
