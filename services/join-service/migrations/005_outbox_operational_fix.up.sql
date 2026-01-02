-- 005_outbox_operational_fix.sql
-- Make outbox operational: pending/sent/dead + attempt + next_retry_at + last_error + message_id(unique)
-- Safe on existing DB with old schema.

BEGIN;

-- 0) Ensure uuid extension exists (for uuid_generate_v4)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1) Ensure columns exist
ALTER TABLE outbox
  ADD COLUMN IF NOT EXISTS message_id UUID,
  ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_error TEXT;

-- 2) Backfill / defaults
-- message_id must exist & be unique for downstream idempotency
UPDATE outbox
SET message_id = uuid_generate_v4()
WHERE message_id IS NULL;

ALTER TABLE outbox
  ALTER COLUMN message_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_outbox_message_id ON outbox (message_id);

-- next_retry_at: make worker gating stable (avoid NULL rows never being picked)
UPDATE outbox
SET next_retry_at = NOW()
WHERE next_retry_at IS NULL;

ALTER TABLE outbox
  ALTER COLUMN next_retry_at SET DEFAULT NOW();

-- 3) Upgrade enum semantics: failed -> dead (no type swap, avoids default cast issues)
DO $$
BEGIN
  -- only if the type exists and has 'failed'
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'outbox_status')
     AND EXISTS (
       SELECT 1
       FROM pg_type t
       JOIN pg_enum e ON t.oid = e.enumtypid
       WHERE t.typname = 'outbox_status'
         AND e.enumlabel = 'failed'
     )
  THEN
     ALTER TYPE outbox_status RENAME VALUE 'failed' TO 'dead';
  END IF;
END$$;

-- 4) Indexes for polling
CREATE INDEX IF NOT EXISTS idx_outbox_status_next_retry
  ON outbox (status, next_retry_at);

CREATE INDEX IF NOT EXISTS idx_outbox_pending_retry_occurred
  ON outbox (status, next_retry_at, occurred_at);

COMMIT;
