-- Allow password-based SSH connections (key optional).

ALTER TABLE ssh_connections
    ADD COLUMN IF NOT EXISTS auth_method TEXT NOT NULL DEFAULT 'key' CHECK (auth_method IN ('key', 'password'));

ALTER TABLE ssh_connections
    ADD COLUMN IF NOT EXISTS password_enc TEXT NULL;

-- Existing installations created 000007 with private_key_enc NOT NULL.
-- Keep it but allow password-only connections by relaxing the constraint.
ALTER TABLE ssh_connections
    ALTER COLUMN private_key_enc DROP NOT NULL;

