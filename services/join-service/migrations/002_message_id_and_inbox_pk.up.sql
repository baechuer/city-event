-- PATH: services/join-service/migrations/002_message_id_and_inbox_pk.sql
-- 1) Add outbox.message_id (dedupe key for downstream consumers)
ALTER TABLE outbox
  ADD COLUMN IF NOT EXISTS message_id UUID;

-- Backfill existing rows (safe if table already has rows)
UPDATE outbox
  SET message_id = uuid_generate_v4()
  WHERE message_id IS NULL;

ALTER TABLE outbox
  ALTER COLUMN message_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_outbox_message_id ON outbox (message_id);

-- 2) Fix processed_messages primary key
-- We want (message_id, handler_name) to be unique so multiple handlers in the SAME service
-- can reuse the same message_id without blocking each other.
ALTER TABLE processed_messages
  DROP CONSTRAINT IF EXISTS processed_messages_pkey;

ALTER TABLE processed_messages
  ADD PRIMARY KEY (message_id, handler_name);
