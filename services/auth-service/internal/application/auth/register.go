package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

func (s *Service) Register(ctx context.Context, email, password string) (RegisterResult, error) {
	if email == "" || password == "" {
		return RegisterResult{}, domain.ErrInvalidField("email/password", "empty")
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return RegisterResult{}, domain.ErrHashFailed(err)
	}

	u := domain.User{
		ID:            uuid.NewString(), // ✅ ADD THIS
		Email:         email,
		PasswordHash:  hash,
		Role:          "user",
		EmailVerified: false,
		Locked:        false,
	}

	created, err := s.users.Create(ctx, u)
	if err != nil {
		return RegisterResult{}, err
	}

	toks, err := s.issueTokens(ctx, created.ID, created.Role)
	if err != nil {
		return RegisterResult{}, err
	}

	// Send verification email (best effort or hard fail? let's hard fail for now to ensure consistency)
	// 1. Generate token
	token, err := newOpaqueToken(32)
	if err != nil {
		return RegisterResult{}, err
	}

	// 2. Save it
	if err := s.ott.Save(ctx, TokenVerifyEmail, token, created.ID, s.verifyEmailTTL); err != nil {
		return RegisterResult{}, err
	}

	// 3. Construct URL
	url := s.verifyEmailBaseURL + token

	// 4. Publish event
	err = s.pub.PublishVerifyEmail(ctx, VerifyEmailEvent{
		UserID: created.ID,
		Email:  created.Email,
		URL:    url,
	})
	if err != nil {
		// In production, we might log this and not fail the registration, allowing user to "Resend Email".
		// But for now, returning error is safer to debug.
		return RegisterResult{}, err
	}

	return RegisterResult{User: created, Tokens: toks}, nil
}
