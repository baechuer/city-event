package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// OAuthIdentityRepo implements auth.OAuthIdentityRepo using PostgreSQL
type OAuthIdentityRepo struct {
	db *sql.DB
}

// NewOAuthIdentityRepo creates a new OAuth identity repository
func NewOAuthIdentityRepo(db *sql.DB) *OAuthIdentityRepo {
	return &OAuthIdentityRepo{db: db}
}

// FindByProviderAndSub finds an OAuth identity by provider and provider user ID
func (r *OAuthIdentityRepo) FindByProviderAndSub(ctx context.Context, provider, providerUserID string) (*domain.OAuthIdentity, error) {
	var identity domain.OAuthIdentity
	var email sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, provider, provider_user_id, email, user_id, created_at
		FROM oauth_identities
		WHERE provider = $1 AND provider_user_id = $2
	`, provider, providerUserID).Scan(
		&identity.ID,
		&identity.Provider,
		&identity.ProviderUserID,
		&email,
		&identity.UserID,
		&identity.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found, return nil without error
		}
		return nil, err
	}

	identity.Email = email.String
	return &identity, nil
}

// FindByUserID finds all OAuth identities for a user
func (r *OAuthIdentityRepo) FindByUserID(ctx context.Context, userID string) ([]domain.OAuthIdentity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider, provider_user_id, email, user_id, created_at
		FROM oauth_identities
		WHERE user_id = $1
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var identities []domain.OAuthIdentity
	for rows.Next() {
		var identity domain.OAuthIdentity
		var email sql.NullString
		if err := rows.Scan(
			&identity.ID,
			&identity.Provider,
			&identity.ProviderUserID,
			&email,
			&identity.UserID,
			&identity.CreatedAt,
		); err != nil {
			return nil, err
		}
		identity.Email = email.String
		identities = append(identities, identity)
	}

	return identities, rows.Err()
}

// Create inserts a new OAuth identity
func (r *OAuthIdentityRepo) Create(ctx context.Context, identity *domain.OAuthIdentity) error {
	var email *string
	if identity.Email != "" {
		email = &identity.Email
	}

	err := r.db.QueryRowContext(ctx, `
		INSERT INTO oauth_identities (id, provider, provider_user_id, email, user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`, identity.ID, identity.Provider, identity.ProviderUserID, email, identity.UserID).Scan(
		&identity.CreatedAt,
	)

	return err
}

// Delete removes an OAuth identity by ID
func (r *OAuthIdentityRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM oauth_identities WHERE id = $1`, id)
	return err
}
