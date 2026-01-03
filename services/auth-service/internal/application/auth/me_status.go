package auth

import (
	"context"
)

type UserStatus struct {
	UserID        string
	Role          string
	Locked        bool
	EmailVerified bool
	HasPassword   bool
}

func (s *Service) GetMyStatus(ctx context.Context, userID string) (UserStatus, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return UserStatus{}, err
	}

	return UserStatus{
		UserID:        u.ID,
		Role:          u.Role,
		Locked:        u.Locked,
		EmailVerified: u.EmailVerified,
		HasPassword:   u.PasswordHash != "",
	}, nil
}

func (s *Service) GetUserStatus(ctx context.Context, targetUserID string) (UserStatus, error) {
	//For administrator checking other users
	u, err := s.users.GetByID(ctx, targetUserID)
	if err != nil {
		return UserStatus{}, err
	}

	return UserStatus{
		UserID:        u.ID,
		Role:          u.Role,
		Locked:        u.Locked,
		EmailVerified: u.EmailVerified,
		HasPassword:   u.PasswordHash != "",
	}, nil
}
