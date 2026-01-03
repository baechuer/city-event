-- Add expires_at column to idempotency_keys for TTL-based cleanup
-- Default: 24 hours from creation
ALTER TABLE idempotency_keys 
ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '24 hours');

-- Index for efficient cleanup queries
CREATE INDEX idx_idempotency_keys_expires_at ON idempotency_keys(expires_at);
