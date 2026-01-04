package domain

import "time"

type User struct {
	ID                string
	Email             string
	PasswordHash      string
	Role              string
	EmailVerified     bool
	Locked            bool
	TokenVersion      int64
	PasswordChangedAt *time.Time
	AvatarImageID     *string // Reference to media_uploads.id
}
