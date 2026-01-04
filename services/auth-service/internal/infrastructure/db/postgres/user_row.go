package postgres

import "time"

type userRow struct {
	ID                string
	Email             string
	PasswordHash      string
	Role              string
	EmailVerified     bool
	Locked            bool
	TokenVersion      int64
	PasswordChangedAt *time.Time
	CreatedAt         time.Time
	AvatarImageID     *string
}
