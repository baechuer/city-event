package auth

import (
	"context"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

/*
UserRepo
--------
Persistence port for users.
Only describes WHAT the auth service needs, not HOW it's stored.
*/
type UserRepo interface {
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByID(ctx context.Context, id string) (domain.User, error)
	Create(ctx context.Context, u domain.User) (domain.User, error)

	// Updates needed by business flows
	UpdatePasswordHash(ctx context.Context, userID string, newHash string) error
	SetEmailVerified(ctx context.Context, userID string) error
	LockUser(ctx context.Context, userID string) error // optional but useful
	UnlockUser(ctx context.Context, userID string) error
	SetRole(ctx context.Context, userID string, role string) error
	CountByRole(ctx context.Context, role string) (int, error)
	GetTokenVersion(ctx context.Context, userID string) (int64, error)
	BumpTokenVersion(ctx context.Context, userID string) (int64, error)
}

/*
PasswordHasher
--------------
Abstracts bcrypt / argon2.
*/
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) error // nil if match
}

/*
TokenSigner
-----------
Issues and verifies access tokens (JWT).
Used by service + auth middleware.
*/
type TokenClaims struct {
	UserID string
	Role   string
	Exp    time.Time
	Ver    int64
}

type TokenSigner interface {
	SignAccessToken(userID string, role string, ttl time.Duration) (string, error)
	VerifyAccessToken(token string) (TokenClaims, error)
}

/*
SessionStore
------------
Refresh token / session management.
Backed by Redis or DB.
*/
type SessionStore interface {
	CreateRefreshToken(ctx context.Context, userID string, ttl time.Duration) (token string, err error)
	RotateRefreshToken(ctx context.Context, oldToken string, ttl time.Duration) (newToken string, err error)
	RevokeRefreshToken(ctx context.Context, token string) error
	RevokeAll(ctx context.Context, userID string) error
	GetUserIDByRefreshToken(ctx context.Context, token string) (string, error)
}

/*
OneTimeTokenStore
-----------------
Opaque one-time tokens for:
- email verification
- password reset
Stored + consumed ONLY by auth-service.
*/
type OneTimeTokenKind string

const (
	TokenVerifyEmail   OneTimeTokenKind = "verify_email"
	TokenPasswordReset OneTimeTokenKind = "password_reset"
)

type OneTimeTokenStore interface {
	Save(ctx context.Context, kind OneTimeTokenKind, token string, userID string, ttl time.Duration) error
	Consume(ctx context.Context, kind OneTimeTokenKind, token string) (userID string, err error)
	Peek(ctx context.Context, kind OneTimeTokenKind, token string) (userID string, err error) // for validate endpoint
}

/*
EventPublisher
--------------
Publishes events to RabbitMQ.
Email-service consumes these and sends emails.
Auth-service does NOT send emails directly.
*/
type EventPublisher interface {
	PublishVerifyEmail(ctx context.Context, evt VerifyEmailEvent) error
	PublishPasswordReset(ctx context.Context, evt PasswordResetEvent) error
}

/*
Event payloads
--------------
Strongly typed messages for MQ.
Email-service does not need to understand tokens.
*/
type VerifyEmailEvent struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	URL    string `json:"url"`
}

type PasswordResetEvent struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	URL    string `json:"url"`
}

/*
OAuthIdentityRepo
-----------------
Persistence port for OAuth identities.
Links external OAuth providers to internal users.
*/
type OAuthIdentityRepo interface {
	FindByProviderAndSub(ctx context.Context, provider, providerUserID string) (*domain.OAuthIdentity, error)
	FindByUserID(ctx context.Context, userID string) ([]domain.OAuthIdentity, error)
	Create(ctx context.Context, identity *domain.OAuthIdentity) error
	Delete(ctx context.Context, id string) error
}

/*
OAuthStateStore
---------------
Stores OAuth state tokens for CSRF protection.
State is one-time use (consume after callback).
*/
type OAuthStateStore interface {
	Create(ctx context.Context, state OAuthStateData) (stateToken string, err error)
	Consume(ctx context.Context, stateToken string) (OAuthStateData, error)
}

type OAuthStateData struct {
	CodeVerifier string // PKCE code verifier
	RedirectTo   string // Where to redirect after login
	Provider     string // OAuth provider name
}
