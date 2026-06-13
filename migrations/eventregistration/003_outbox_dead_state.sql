ALTER TABLE outbox_messages
    DROP CONSTRAINT IF EXISTS outbox_messages_status_check;

ALTER TABLE outbox_messages
    ADD CONSTRAINT outbox_messages_status_check
    CHECK (status IN ('PENDING', 'SENT', 'FAILED', 'DEAD'));

DROP INDEX IF EXISTS idx_outbox_messages_pending;

CREATE INDEX IF NOT EXISTS idx_outbox_messages_pending
    ON outbox_messages(status, available_at, created_at)
    WHERE status IN ('PENDING', 'FAILED');
