-- Add status enum and retry scheduling to event_outbox
CREATE TYPE outbox_status AS ENUM ('pending', 'sent', 'failed', 'dead');

ALTER TABLE event_outbox
ADD COLUMN status outbox_status NOT NULL DEFAULT 'pending',
ADD COLUMN next_retry_at TIMESTAMPTZ;

-- Backfill: if sent_at IS NOT NULL -> sent, else pending
UPDATE event_outbox SET status = 'sent' WHERE sent_at IS NOT NULL;
-- Initialize next_retry_at for pending items
UPDATE event_outbox SET next_retry_at = created_at WHERE status = 'pending';

-- Drop old index
DROP INDEX IF EXISTS idx_event_outbox_unsent;

-- Create optimized polling index
CREATE INDEX idx_event_outbox_pending_retry 
ON event_outbox (next_retry_at ASC, created_at ASC) 
WHERE status = 'pending';
