package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrInvalidEmail       = errors.New("invalid email")
	ErrWeakPassword       = errors.New("weak password")
	ErrInvalidDisplayName = errors.New("invalid display name")
)

type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type PublicUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type RegisterCommand struct {
	Email       string
	Password    string
	DisplayName string
}

type LoginCommand struct {
	Email    string
	Password string
}

func (u User) Public() PublicUser {
	return PublicUser{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidateEmail(email string) error {
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, ".") {
		return ErrInvalidEmail
	}
	return nil
}

func ValidatePassword(password string) error {
	if len(password) < 12 {
		return ErrWeakPassword
	}

	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

func ValidateDisplayName(displayName string) error {
	name := strings.TrimSpace(displayName)
	if name == "" || len(name) > 80 {
		return ErrInvalidDisplayName
	}
	return nil
}

func NewUser(email, displayName, passwordHash string, now time.Time) (User, error) {
	email = NormalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	if err := ValidateEmail(email); err != nil {
		return User{}, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return User{}, err
	}
	if passwordHash == "" {
		return User{}, ErrWeakPassword
	}

	return User{
		ID:           NewID(),
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    now.UTC(),
		UpdatedAt:    now.UTC(),
	}, nil
}

func NewID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
