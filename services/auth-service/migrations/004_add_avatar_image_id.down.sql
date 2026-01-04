-- Remove avatar_image_id column from users table
ALTER TABLE users DROP COLUMN IF EXISTS avatar_image_id;
