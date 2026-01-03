DROP INDEX IF EXISTS idx_events_autocomplete;
DROP INDEX IF EXISTS idx_events_city_norm_prefix;
ALTER TABLE events DROP COLUMN IF EXISTS city_norm;
