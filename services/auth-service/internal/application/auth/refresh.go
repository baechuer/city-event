package auth

import (
	"context"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// Refresh rotates a refresh token and issues a new access token.
// Rotation rule: old refresh token becomes invalid once used successfully.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthTokens, domain.User, error) {
	if refreshToken == "" {
		return AuthTokens{}, domain.User{}, domain.ErrRefreshTokenInvalid()
	}

	// Map refresh token -> user
	userID, err := s.sessions.GetUserIDByRefreshToken(ctx, refreshToken)
	if err != nil {
		// Hide details: treat as invalid
		return AuthTokens{}, domain.User{}, domain.ErrRefreshTokenInvalid()
	}

	// Load user to get role (and optional policy checks)
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		// If user is gone, treat as invalid session
		return AuthTokens{}, domain.User{}, domain.ErrRefreshTokenInvalid()
	}

	// Optional policy checks (enable if desired)
	if u.Locked {
		return AuthTokens{}, domain.User{}, domain.ErrAccountLocked()
	}
	// if !u.EmailVerified {
	// 	return AuthTokens{}, domain.User{}, domain.ErrEmailNotVerified()
	// }

	// Rotate refresh token
	newRefresh, err := s.sessions.RotateRefreshToken(ctx, refreshToken, s.refreshTTL)
	if err != nil {
		// If you later implement reuse detection, map to ErrRefreshTokenReused()
		return AuthTokens{}, domain.User{}, domain.ErrRefreshTokenInvalid()
	}

	// Issue a new access token
	access, err := s.signer.SignAccessToken(u.ID, u.Role, s.accessTTL)
	if err != nil {
		return AuthTokens{}, domain.User{}, domain.ErrTokenSignFailed(err)
	}

	return AuthTokens{
		AccessToken:  access,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, u, nil
}
