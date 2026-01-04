-- Add cover_image_ids column to events table (JSON array, max 2 elements)
ALTER TABLE events ADD COLUMN IF NOT EXISTS cover_image_ids JSONB DEFAULT '[]';

COMMENT ON COLUMN events.cover_image_ids IS 'Array of media_uploads.id references for event cover images (max 2)';
