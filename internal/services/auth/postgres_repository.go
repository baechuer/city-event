package auth

import (
	"context"
	"errors"
	"time"

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
		INSERT INTO auth_users (id, email, display_name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, user.ID, NormalizeEmail(user.Email), user.DisplayName, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
	if isUniqueViolation(err) {
		return ErrDuplicateEmail
	}
	return err
}

func (r *PostgresRepository) FindUserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, password_hash, created_at, updated_at
		FROM auth_users
		WHERE email = $1
	`, NormalizeEmail(email)))
}

func (r *PostgresRepository) FindUserByID(ctx context.Context, id string) (User, error) {
	return scanUser(r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, password_hash, created_at, updated_at
		FROM auth_users
		WHERE id = $1
	`, id))
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

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
