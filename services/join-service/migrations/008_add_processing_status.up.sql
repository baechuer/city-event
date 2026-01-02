-- Add 'processing' status to outbox_status enum
-- This is needed for the outbox worker to mark messages as being processed

BEGIN;

-- Add 'processing' to the outbox_status enum
ALTER TYPE outbox_status ADD VALUE IF NOT EXISTS 'processing';

COMMIT;
