-- Add index on users.role for admin queries (CountByRole)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_role ON users(role);
