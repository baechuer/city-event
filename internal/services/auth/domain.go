package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

var (
	ErrInvalidEmail       = errors.New("invalid email")
	ErrWeakPassword       = errors.New("weak password")
	ErrInvalidDisplayName = errors.New("invalid display name")
	ErrInvalidRole        = errors.New("invalid role")
)

type User struct {
	ID           string
	Email        string
	DisplayName  string
	Role         identity.Role
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type PublicUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type RegisterCommand struct {
	Email       string
	Password    string
	DisplayName string
	Role        identity.Role
}

type LoginCommand struct {
	Email    string
	Password string
}

type SeedAdminCommand struct {
	Email       string
	Password    string
	DisplayName string
}

type UpdateRoleCommand struct {
	ActorUserID  string
	TargetUserID string
	Role         identity.Role
}

func (u User) Public() PublicUser {
	return PublicUser{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        string(u.Role),
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

func ValidateRole(role identity.Role) error {
	role = identity.NormalizeRole(string(role))
	if !identity.ValidRole(role) {
		return ErrInvalidRole
	}
	return nil
}

func NewUser(email, displayName string, role identity.Role, passwordHash string, now time.Time) (User, error) {
	email = NormalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	role = identity.NormalizeRole(string(role))
	if err := ValidateEmail(email); err != nil {
		return User{}, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return User{}, err
	}
	if err := ValidateRole(role); err != nil {
		return User{}, err
	}
	if passwordHash == "" {
		return User{}, ErrWeakPassword
	}

	return User{
		ID:           NewID(),
		Email:        email,
		DisplayName:  displayName,
		Role:         role,
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
