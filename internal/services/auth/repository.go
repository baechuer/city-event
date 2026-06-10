package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrDuplicateEmail = errors.New("duplicate email")
)

type Repository interface {
	CreateUser(context.Context, User) error
	FindUserByEmail(context.Context, string) (User, error)
	FindUserByID(context.Context, string) (User, error)
	UpdateUserRole(context.Context, string, identity.Role) (User, error)
	RevokeToken(context.Context, string, string, time.Time) error
	IsTokenRevoked(context.Context, string) (bool, error)
}

type MemoryRepository struct {
	mu            sync.RWMutex
	usersByID     map[string]User
	usersByEmail  map[string]User
	revokedTokens map[string]revokedToken
}

type revokedToken struct {
	UserID    string
	ExpiresAt time.Time
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:     map[string]User{},
		usersByEmail:  map[string]User{},
		revokedTokens: map[string]revokedToken{},
	}
}

func (r *MemoryRepository) CreateUser(_ context.Context, user User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	email := NormalizeEmail(user.Email)
	if _, exists := r.usersByEmail[email]; exists {
		return ErrDuplicateEmail
	}
	user.Email = email
	user.Role = identity.NormalizeRole(string(user.Role))
	r.usersByID[user.ID] = user
	r.usersByEmail[email] = user
	return nil
}

func (r *MemoryRepository) FindUserByEmail(_ context.Context, email string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.usersByEmail[NormalizeEmail(email)]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (r *MemoryRepository) FindUserByID(_ context.Context, id string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.usersByID[id]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (r *MemoryRepository) UpdateUserRole(_ context.Context, id string, role identity.Role) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.usersByID[id]
	if !exists {
		return User{}, ErrUserNotFound
	}
	user.Role = identity.NormalizeRole(string(role))
	user.UpdatedAt = time.Now().UTC()
	r.usersByID[id] = user
	r.usersByEmail[NormalizeEmail(user.Email)] = user
	return user, nil
}

func (r *MemoryRepository) RevokeToken(_ context.Context, tokenID, userID string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.revokedTokens[tokenID] = revokedToken{UserID: userID, ExpiresAt: expiresAt}
	return nil
}

func (r *MemoryRepository) IsTokenRevoked(_ context.Context, tokenID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	token, exists := r.revokedTokens[tokenID]
	if !exists {
		return false, nil
	}
	if time.Now().After(token.ExpiresAt) {
		return false, nil
	}
	return true, nil
}

func (r *MemoryRepository) CountUsersByEmail(email string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.usersByEmail[NormalizeEmail(email)]; exists {
		return 1
	}
	return 0
}
