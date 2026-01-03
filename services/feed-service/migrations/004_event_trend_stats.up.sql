-- event_trend_stats: pre-aggregated trending signals (24h + 7d)

CREATE TABLE event_trend_stats (
    event_id UUID PRIMARY KEY,
    city TEXT NOT NULL,
    join_users_24h INT DEFAULT 0,
    join_users_7d INT DEFAULT 0,
    view_users_24h INT DEFAULT 0,
    view_users_7d INT DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX ix_trend_stats_city ON event_trend_stats(city);
