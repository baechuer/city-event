-- Add avatar_image_id column to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_image_id UUID;

COMMENT ON COLUMN users.avatar_image_id IS 'Reference to media_uploads.id for user avatar';
