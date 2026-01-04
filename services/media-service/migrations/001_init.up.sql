-- Media uploads table
CREATE TABLE IF NOT EXISTS media_uploads (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    purpose VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    raw_object_key VARCHAR(256),
    derived_keys JSONB DEFAULT '{}',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for querying by owner and purpose
CREATE INDEX IF NOT EXISTS idx_media_uploads_owner_purpose 
ON media_uploads(owner_id, purpose, status);

-- Index for finding pending uploads (cleanup)
CREATE INDEX IF NOT EXISTS idx_media_uploads_status 
ON media_uploads(status) WHERE status IN ('PENDING', 'UPLOADED', 'PROCESSING');
