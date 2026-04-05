ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS public_origin_override TEXT;
