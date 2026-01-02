-- 006_reads_and_moderation.sql

-- 1) joins: keep history + moderation metadata
ALTER TABLE joins
  ADD COLUMN IF NOT EXISTS canceled_by UUID NULL,
  ADD COLUMN IF NOT EXISTS canceled_reason TEXT NULL,
  ADD COLUMN IF NOT EXISTS rejected_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS rejected_by UUID NULL,
  ADD COLUMN IF NOT EXISTS rejected_reason TEXT NULL;

-- 2) bans: event-level bans to prevent join
CREATE TABLE IF NOT EXISTS event_bans (
  event_id UUID NOT NULL,
  user_id  UUID NOT NULL,
  actor_id UUID NOT NULL,
  reason   TEXT NULL,
  expires_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (event_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_event_bans_event ON event_bans (event_id);
CREATE INDEX IF NOT EXISTS idx_event_bans_user  ON event_bans (user_id);

-- 3) indexes for keyset pages
-- me/joins: WHERE user_id=? ORDER BY created_at DESC, id DESC
CREATE INDEX IF NOT EXISTS idx_joins_user_created_id_desc
  ON joins (user_id, created_at DESC, id DESC);

-- participants/waitlist: WHERE event_id=? AND status=? ORDER BY created_at ASC, id ASC
CREATE INDEX IF NOT EXISTS idx_joins_event_status_created_id_asc
  ON joins (event_id, status, created_at ASC, id ASC);
