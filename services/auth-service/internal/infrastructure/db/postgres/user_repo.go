package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// ---------- helpers ----------

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (r *UserRepo) scanUserRow(row *sql.Row) (userRow, error) {
	var ur userRow
	err := row.Scan(
		&ur.ID,
		&ur.Email,
		&ur.PasswordHash,
		&ur.Role,
		&ur.EmailVerified,
		&ur.Locked,
		&ur.TokenVersion,
		&ur.PasswordChangedAt,
		&ur.CreatedAt,
		&ur.AvatarImageID,
	)
	return ur, err
}

func toDomainUser(ur userRow) domain.User {

	return domain.User{
		ID:            ur.ID,
		Email:         ur.Email,
		PasswordHash:  ur.PasswordHash,
		Role:          ur.Role,
		EmailVerified: ur.EmailVerified,
		Locked:        ur.Locked,
		// TokenVersion:     ur.TokenVersion,
		// PasswordChangedAt: ur.PasswordChangedAt,
		AvatarImageID: ur.AvatarImageID,
	}
}

// ---------- auth.UserRepo ----------

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	email = normalizeEmail(email)
	if email == "" {
		return domain.User{}, domain.ErrMissingField("email")
	}

	const q = `
SELECT id, email, password_hash, role, email_verified, locked, token_version, password_changed_at, created_at, avatar_image_id
FROM users
WHERE email = $1
LIMIT 1;
`
	ur, err := r.scanUserRow(r.db.QueryRowContext(ctx, q, email))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound()
		}
		return domain.User{}, domain.ErrDBUnavailable(err)
	}
	return toDomainUser(ur), nil
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (domain.User, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.User{}, domain.ErrMissingField("id")
	}

	const q = `
SELECT id, email, password_hash, role, email_verified, locked, token_version, password_changed_at, created_at, avatar_image_id
FROM users
WHERE id = $1
LIMIT 1;
`
	ur, err := r.scanUserRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound()
		}
		return domain.User{}, domain.ErrDBUnavailable(err)
	}
	return toDomainUser(ur), nil
}

func (r *UserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	u.Email = normalizeEmail(u.Email)
	if u.ID == "" {
		return domain.User{}, domain.ErrMissingField("id")
	}
	if u.Email == "" {
		return domain.User{}, domain.ErrMissingField("email")
	}
	// PasswordHash can be empty for OAuth users
	if u.Role == "" {
		u.Role = string(domain.RoleUser)
	}

	const q = `
INSERT INTO users (id, email, password_hash, role, email_verified, locked)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, email, password_hash, role, email_verified, locked, token_version, password_changed_at, created_at, avatar_image_id;
`

	var ur userRow
	err := r.db.QueryRowContext(ctx, q,
		u.ID, u.Email, u.PasswordHash, u.Role, u.EmailVerified, u.Locked,
	).Scan(
		&ur.ID,
		&ur.Email,
		&ur.PasswordHash,
		&ur.Role,
		&ur.EmailVerified,
		&ur.Locked,
		&ur.TokenVersion,
		&ur.PasswordChangedAt,
		&ur.CreatedAt,
	)
	if err != nil {

		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return domain.User{}, domain.ErrEmailAlreadyExists()
		}
		return domain.User{}, domain.ErrDBUnavailable(err)
	}
	return toDomainUser(ur), nil
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, userID string, newHash string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	if newHash == "" {
		return domain.ErrMissingField("password_hash")
	}

	const q = `
UPDATE users
SET password_hash = $2,
    password_changed_at = NOW()
WHERE id = $1;
`
	res, err := r.db.ExecContext(ctx, q, userID, newHash)
	if err != nil {
		return domain.ErrDBUnavailable(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrUserNotFound()
	}
	return nil
}

func (r *UserRepo) SetEmailVerified(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}

	const q = `
UPDATE users
SET email_verified = TRUE
WHERE id = $1;
`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return domain.ErrDBUnavailable(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrUserNotFound()
	}
	return nil
}

func (r *UserRepo) LockUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}

	const q = `
UPDATE users
SET locked = TRUE
WHERE id = $1;
`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return domain.ErrDBUnavailable(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrUserNotFound()
	}
	return nil
}

func (r *UserRepo) UnlockUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingField("user_id")
	}

	const q = `
UPDATE users
SET locked = FALSE
WHERE id = $1;
`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return domain.ErrDBUnavailable(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrUserNotFound()
	}
	return nil
}

func (r *UserRepo) SetRole(ctx context.Context, userID string, role string) error {
	userID = strings.TrimSpace(userID)
	role = strings.TrimSpace(role)

	if userID == "" {
		return domain.ErrMissingField("user_id")
	}
	if !domain.IsValidRole(role) {
		return domain.ErrInvalidRole(role)
	}

	const q = `
UPDATE users
SET role = $2
WHERE id = $1;
`
	res, err := r.db.ExecContext(ctx, q, userID, role)
	if err != nil {
		return domain.ErrDBUnavailable(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrUserNotFound()
	}
	return nil
}

func (r *UserRepo) CountByRole(ctx context.Context, role string) (int, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return 0, domain.ErrMissingField("role")
	}
	if !domain.IsValidRole(role) {
		return 0, domain.ErrInvalidRole(role)
	}

	const q = `SELECT COUNT(1) FROM users WHERE role = $1;`

	var n int
	if err := r.db.QueryRowContext(ctx, q, role).Scan(&n); err != nil {
		return 0, domain.ErrDBUnavailable(err)
	}
	return n, nil
}
func (r *UserRepo) GetTokenVersion(ctx context.Context, userID string) (int64, error) {
	const q = `
SELECT token_version
FROM users
WHERE id = $1
`
	var ver int64
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&ver)
	if err != nil {
		if isNoRows(err) {
			return 0, domain.ErrUserNotFound()
		}
		return 0, domain.ErrInternal(err)
	}
	return ver, nil
}
func (r *UserRepo) BumpTokenVersion(ctx context.Context, userID string) (int64, error) {
	const q = `
UPDATE users
SET token_version = token_version + 1,
    password_changed_at = NOW()
WHERE id = $1
RETURNING token_version
`
	var newVer int64
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&newVer)
	if err != nil {
		if isNoRows(err) {
			return 0, domain.ErrUserNotFound()
		}
		return 0, domain.ErrInternal(err)
	}
	return newVer, nil
}

// UpdateAvatarImageID updates the user's avatar image ID.
// Returns the previous avatar_image_id for cleanup purposes.
func (r *UserRepo) UpdateAvatarImageID(ctx context.Context, userID string, avatarImageID *string) (*string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, domain.ErrMissingField("user_id")
	}

	const q = `
UPDATE users
SET avatar_image_id = $2
WHERE id = $1
RETURNING (SELECT avatar_image_id FROM users WHERE id = $1);
`
	var prevAvatarID sql.NullString
	err := r.db.QueryRowContext(ctx, q, userID, avatarImageID).Scan(&prevAvatarID)
	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrUserNotFound()
		}
		return nil, domain.ErrDBUnavailable(err)
	}

	if prevAvatarID.Valid {
		return &prevAvatarID.String, nil
	}
	return nil, nil
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
