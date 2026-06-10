package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, user User) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth_users (id, email, display_name, role, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, user.ID, NormalizeEmail(user.Email), user.DisplayName, user.Role, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
	if isUniqueViolation(err) {
		return ErrDuplicateEmail
	}
	return err
}

func (r *PostgresRepository) FindUserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, role, password_hash, created_at, updated_at
		FROM auth_users
		WHERE email = $1
	`, NormalizeEmail(email)))
}

func (r *PostgresRepository) FindUserByID(ctx context.Context, id string) (User, error) {
	return scanUser(r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, role, password_hash, created_at, updated_at
		FROM auth_users
		WHERE id = $1
	`, id))
}

func (r *PostgresRepository) UpdateUserRole(ctx context.Context, id string, role identity.Role) (User, error) {
	return scanUser(r.pool.QueryRow(ctx, `
		UPDATE auth_users
		SET role = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, email, display_name, role, password_hash, created_at, updated_at
	`, id, identity.NormalizeRole(string(role))))
}

func (r *PostgresRepository) RevokeToken(ctx context.Context, tokenID, userID string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO revoked_tokens (token_id, user_id, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (token_id) DO NOTHING
	`, tokenID, userID, expiresAt)
	return err
}

func (r *PostgresRepository) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM revoked_tokens
			WHERE token_id = $1 AND expires_at > now()
		)
	`, tokenID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) CreateRefreshSession(ctx context.Context, session RefreshSession) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (token_hash, user_id, family_id, replaced_by_hash, expires_at, revoked_at, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
	`, session.TokenHash, session.UserID, session.FamilyID, session.ReplacedByHash, session.ExpiresAt, session.RevokedAt, session.CreatedAt, session.UpdatedAt)
	return err
}

func (r *PostgresRepository) RotateRefreshSession(ctx context.Context, oldHash, newHash string, newExpiresAt, now time.Time) (User, RefreshSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, RefreshSession{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, user, err := refreshSessionForUpdate(ctx, tx, oldHash)
	if err != nil {
		return User{}, RefreshSession{}, err
	}
	if !current.ExpiresAt.After(now) {
		return User{}, RefreshSession{}, ErrUnauthorized
	}
	if current.RevokedAt != nil || current.ReplacedByHash != "" {
		if err := revokeRefreshFamilyTx(ctx, tx, current.FamilyID, now); err != nil {
			return User{}, RefreshSession{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return User{}, RefreshSession{}, err
		}
		return User{}, RefreshSession{}, ErrRefreshReuse
	}

	_, err = tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, replaced_by_hash = $3, updated_at = $2
		WHERE token_hash = $1
	`, oldHash, now, newHash)
	if err != nil {
		return User{}, RefreshSession{}, err
	}

	next := RefreshSession{
		TokenHash: newHash,
		UserID:    current.UserID,
		FamilyID:  current.FamilyID,
		ExpiresAt: newExpiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (token_hash, user_id, family_id, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, next.TokenHash, next.UserID, next.FamilyID, next.ExpiresAt, next.CreatedAt, next.UpdatedAt); err != nil {
		return User{}, RefreshSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, RefreshSession{}, err
	}
	return user, next, nil
}

func (r *PostgresRepository) RevokeRefreshSession(ctx context.Context, tokenHash string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, _, err := refreshSessionForUpdate(ctx, tx, tokenHash)
	if errors.Is(err, ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := revokeRefreshFamilyTx(ctx, tx, current.FamilyID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	user.Role = identity.NormalizeRole(string(user.Role))
	return user, err
}

func refreshSessionForUpdate(ctx context.Context, tx pgx.Tx, tokenHash string) (RefreshSession, User, error) {
	var session RefreshSession
	var user User
	var replacedBy sql.NullString
	var revokedAt sql.NullTime
	err := tx.QueryRow(ctx, `
		SELECT rt.token_hash, rt.user_id, rt.family_id, rt.replaced_by_hash, rt.expires_at, rt.revoked_at, rt.created_at, rt.updated_at,
		       au.id, au.email, au.display_name, au.role, au.password_hash, au.created_at, au.updated_at
		FROM refresh_tokens rt
		JOIN auth_users au ON au.id = rt.user_id
		WHERE rt.token_hash = $1
		FOR UPDATE OF rt
	`, tokenHash).Scan(
		&session.TokenHash,
		&session.UserID,
		&session.FamilyID,
		&replacedBy,
		&session.ExpiresAt,
		&revokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshSession{}, User{}, ErrUnauthorized
	}
	if err != nil {
		return RefreshSession{}, User{}, err
	}
	if replacedBy.Valid {
		session.ReplacedByHash = replacedBy.String
	}
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}
	user.Role = identity.NormalizeRole(string(user.Role))
	return session, user, nil
}

func revokeRefreshFamilyTx(ctx context.Context, tx pgx.Tx, familyID string, now time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, $2), updated_at = $2
		WHERE family_id = $1
	`, familyID, now)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
