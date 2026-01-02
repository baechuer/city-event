-- 003_event_canceled_expire.sql

-- 1) joins: add expired metadata
ALTER TABLE joins
  ADD COLUMN IF NOT EXISTS expired_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS expired_reason TEXT NULL;

-- 2) (Optional) If you want strict status domain, you can enforce via CHECK.
-- Commented out because you might still be iterating states.
-- ALTER TABLE joins
--   ADD CONSTRAINT joins_status_check
--   CHECK (status IN ('active','waitlisted','canceled','expired'));

-- 3) Indexes to keep bulk expire + waitlist promotion fast
-- For: UPDATE joins WHERE event_id=? AND status IN (...)
-- For: SELECT waitlisted ORDER BY created_at LIMIT 1
CREATE INDEX IF NOT EXISTS idx_joins_event_status_created_at
  ON joins (event_id, status, created_at);

-- If you query user history frequently
CREATE INDEX IF NOT EXISTS idx_joins_user_created_at
  ON joins (user_id, created_at);

-- Outbox worker query:
-- WHERE status='pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
-- ORDER BY occurred_at ASC LIMIT 10
CREATE INDEX IF NOT EXISTS idx_outbox_pending_retry_occurred
  ON outbox (status, next_retry_at, occurred_at);
