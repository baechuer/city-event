-- Add city_norm column for normalized city storage
ALTER TABLE events ADD COLUMN city_norm TEXT;

-- Backfill existing data (lowercase + trim)
UPDATE events SET city_norm = LOWER(TRIM(city));

-- Make it NOT NULL after backfill
ALTER TABLE events ALTER COLUMN city_norm SET NOT NULL;

-- Add index for autocomplete (prefix search)
CREATE INDEX idx_events_city_norm_prefix ON events (city_norm text_pattern_ops);

-- Add composite index for autocomplete query (status + time + city_norm)
CREATE INDEX idx_events_autocomplete 
  ON events (status, start_time, city_norm text_pattern_ops) 
  WHERE status = 'published';
