//go:build integration

package infra

import (
	"context"
	"database/sql"
)

// EnsureAuthSchema：
// - 建表 users（若不存在）
// - 补齐 integration tests 会触发的字段（email_verified/locked/password_changed_at/token_version 等）
//
// 你真实项目如果有 migrations，最终建议：
// ✅ integration 环境里跑 migration
// 这里先保证你“能跑通 tests”，并且不会再手动 ALTER 一路补列。
func EnsureAuthSchema(ctx context.Context, db *sql.DB) error {
	// 1) create table if not exists (宽松字段集合)
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT,

  role TEXT NOT NULL DEFAULT 'user',
  token_version BIGINT NOT NULL DEFAULT 0,

  email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  locked BOOLEAN NOT NULL DEFAULT FALSE,

  password_changed_at TIMESTAMPTZ NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`)
	if err != nil {
		return err
	}

	// 2) patch columns if missing (幂等)
	stmts := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS locked BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMPTZ NULL;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;`,
		// relax password_hash not null if it exists as not null (optional, pg doesn't support ALTER COLUMN drop not null in ADD COLUMN)
		`ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;`,
	}

	for _, s := range stmts {
		if _, e := db.ExecContext(ctx, s); e != nil {
			// ignore error if column 'password_hash' doesn't exist? No, generic exec.
			// ALTER COLUMN DROP NOT NULL is safe even if already nullable.
			// But if column doesn't exist it fails. Wait, ADD COLUMN creates it.
			// So order matters.
			return e
		}
	}

	// 3) Create oauth_identities
	_, err = db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS oauth_identities (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);
`)
	return err
}
