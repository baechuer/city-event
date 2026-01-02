CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,
    user_id UUID NOT NULL,
    event_id UUID NOT NULL,
    action TEXT NOT NULL, -- 'join' or 'cancel'
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
