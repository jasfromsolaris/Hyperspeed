ALTER TABLE organizations
  DROP COLUMN IF EXISTS gifted_subdomain_slug,
  DROP COLUMN IF EXISTS intended_public_url;
