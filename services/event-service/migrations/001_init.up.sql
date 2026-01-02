CREATE TABLE IF NOT EXISTS events (
  id UUID PRIMARY KEY,
  owner_id TEXT NOT NULL,

  title TEXT NOT NULL,
  description TEXT NOT NULL,

  city TEXT NOT NULL,
  category TEXT NOT NULL,

  start_time TIMESTAMPTZ NOT NULL,
  end_time   TIMESTAMPTZ NOT NULL,

  capacity INT NOT NULL DEFAULT 0, -- 0 = unlimited
  status TEXT NOT NULL,            -- draft|published|canceled

  published_at TIMESTAMPTZ NULL,
  canceled_at  TIMESTAMPTZ NULL,

  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_status_start
  ON events (status, start_time);

CREATE INDEX IF NOT EXISTS idx_events_city_start
  ON events (city, start_time);

CREATE INDEX IF NOT EXISTS idx_events_owner_created
  ON events (owner_id, created_at DESC);
