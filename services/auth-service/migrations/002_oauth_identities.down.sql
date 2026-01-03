-- Revert OAuth identities migration
DROP INDEX IF EXISTS idx_oauth_identities_email;
DROP INDEX IF EXISTS idx_oauth_identities_user_id;
DROP TABLE IF EXISTS oauth_identities;

-- Note: We don't revert username column or password_hash nullability
-- as it could break existing data
