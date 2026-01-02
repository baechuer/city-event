-- Outbox table for durable event publishing (event-service)
-- Each row represents exactly one message; message_id is the dedupe key for consumers.
CREATE TABLE IF NOT EXISTS event_outbox (
  id BIGSERIAL PRIMARY KEY,
  message_id UUID NOT NULL UNIQUE,
  routing_key TEXT NOT NULL,
  body JSONB NOT NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at    TIMESTAMPTZ NULL,

  attempts   INT NOT NULL DEFAULT 0,
  last_error TEXT NULL
);

-- Speed up polling for unsent rows
CREATE INDEX IF NOT EXISTS idx_event_outbox_unsent
  ON event_outbox (id)
  WHERE sent_at IS NULL;
