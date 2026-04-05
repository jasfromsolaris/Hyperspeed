-- Add a user-friendly display name.

ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT;

-- Optional backfill: use email local-part where display_name is missing.
UPDATE users
SET display_name = split_part(email, '@', 1)
WHERE (display_name IS NULL OR btrim(display_name) = '') AND email IS NOT NULL;

