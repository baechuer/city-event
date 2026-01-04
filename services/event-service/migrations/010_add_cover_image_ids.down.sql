-- Remove cover_image_ids column from events table
ALTER TABLE events DROP COLUMN IF EXISTS cover_image_ids;
