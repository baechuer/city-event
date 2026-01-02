

BEGIN;

ALTER TABLE outbox 
  ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_error TEXT;


ALTER TABLE outbox ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
UPDATE outbox SET next_retry_at = NOW() WHERE next_retry_at IS NULL;
ALTER TABLE outbox ALTER COLUMN next_retry_at SET DEFAULT NOW();

ALTER TABLE outbox ALTER COLUMN status DROP DEFAULT;

CREATE INDEX IF NOT EXISTS idx_outbox_status_next_retry
  ON outbox (status, next_retry_at);

COMMIT;