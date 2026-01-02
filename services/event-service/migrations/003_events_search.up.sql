-- 0002_events_search.sql
-- Full-text search (FTS) for events: weighted tsvector + partial GIN + keyset helper index

BEGIN;

-- 1) Add generated weighted tsvector column (Postgres 12+)
-- Title gets higher weight than description.
ALTER TABLE events
  ADD COLUMN IF NOT EXISTS search_vector tsvector
  GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(description, '')), 'B')
  ) STORED;

-- 2) Partial GIN index only for published rows (smaller + faster)
CREATE INDEX IF NOT EXISTS idx_events_search_vector_published_gin
  ON events
  USING GIN (search_vector)
  WHERE status = 'published';

-- 3) Keyset helper index for public list ordering / pagination
-- (you order by start_time ASC, id ASC; city filter is common)
CREATE INDEX IF NOT EXISTS idx_events_public_city_time_id
  ON events (city, start_time, id)
  WHERE status = 'published';

-- 4) (Optional but useful) If you often filter by category too:
-- CREATE INDEX IF NOT EXISTS idx_events_public_city_category_time_id
--   ON events (city, category, start_time, id)
--   WHERE status = 'published';

COMMIT;
