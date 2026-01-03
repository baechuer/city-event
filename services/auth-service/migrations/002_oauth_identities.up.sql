-- OAuth identities table for social login
CREATE TABLE oauth_identities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider TEXT NOT NULL,                -- 'google', 'github', 'discord'
  provider_user_id TEXT NOT NULL,        -- sub/id from provider
  email TEXT,                            -- cached for lookup
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  
  UNIQUE(provider, provider_user_id)     -- one identity per provider
);

CREATE INDEX idx_oauth_identities_user_id ON oauth_identities(user_id);
CREATE INDEX idx_oauth_identities_email ON oauth_identities(email);

-- Add username column for display name (from OAuth providers)
ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT;

-- Make password_hash nullable for OAuth-only users
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
