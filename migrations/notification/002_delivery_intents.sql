ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS notifications_status_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_status_check
    CHECK (status IN ('PENDING', 'PROCESSING', 'SENT', 'FAILED'));

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

UPDATE notifications
SET idempotency_key = message_id
WHERE idempotency_key = '';

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS delivery_attempts INTEGER NOT NULL DEFAULT 0;

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_notifications_delivery_claim
    ON notifications(status, next_attempt_at, locked_until, created_at)
    WHERE status IN ('PENDING', 'PROCESSING', 'FAILED');
