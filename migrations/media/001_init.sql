CREATE TABLE IF NOT EXISTS media_assets (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    uploader_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    bucket TEXT NOT NULL,
    object_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('UPLOADING', 'UPLOADED', 'PROCESSING', 'READY', 'FAILED')),
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_media_assets_event
    ON media_assets(event_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_media_assets_worker
    ON media_assets(status, updated_at, id)
    WHERE status = 'UPLOADED';
