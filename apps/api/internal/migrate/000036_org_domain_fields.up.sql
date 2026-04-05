ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS intended_public_url TEXT,
  ADD COLUMN IF NOT EXISTS gifted_subdomain_slug TEXT;
