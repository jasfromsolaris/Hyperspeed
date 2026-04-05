-- Presence: track last-seen timestamps for users.

ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Ensure existing users get a value.
UPDATE users SET last_seen_at = now() WHERE last_seen_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_last_seen_at ON users(last_seen_at);

