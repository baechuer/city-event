-- 1) Public list: status + start_time (time-sorted published feed)
CREATE INDEX IF NOT EXISTS idx_events_public_time
ON events (status, start_time, id);

-- 2) city filter + time ordering
CREATE INDEX IF NOT EXISTS idx_events_city_time
ON events (status, city, start_time, id);

-- 3) category filter + time ordering
CREATE INDEX IF NOT EXISTS idx_events_category_time
ON events (status, category, start_time, id);

-- 4) city + category + time 
CREATE INDEX IF NOT EXISTS idx_events_city_category_time
ON events (status, city, category, start_time, id);

CREATE INDEX IF NOT EXISTS idx_events_public_keyset
ON events (start_time ASC, id ASC)
WHERE status = 'published';
