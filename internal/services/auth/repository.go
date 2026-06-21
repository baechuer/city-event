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
	ErrRefreshReuse   = errors.New("refresh token reuse detected")
)

type Repository interface {
	CreateUser(context.Context, User) error
	FindUserByEmail(context.Context, string) (User, error)
	FindUserByID(context.Context, string) (User, error)
	UpdateUserRole(context.Context, string, identity.Role) (User, error)
	UpdateUserRoleWithAudit(context.Context, string, identity.Role, AuditEvent) (User, error)
	RevokeToken(context.Context, string, string, time.Time) error
	IsTokenRevoked(context.Context, string) (bool, error)
	CreateRefreshSession(context.Context, RefreshSession) error
	RotateRefreshSession(context.Context, string, string, time.Time, time.Time) (User, RefreshSession, error)
	RevokeRefreshSession(context.Context, string, time.Time) error
}

type AuditEvent struct {
	ID            string
	ActorUserID   string
	Action        string
	TargetUserID  string
	TargetType    string
	Result        string
	CorrelationID string
	Metadata      map[string]any
	CreatedAt     time.Time
}

type MemoryRepository struct {
	mu            sync.RWMutex
	usersByID     map[string]User
	usersByEmail  map[string]User
	revokedTokens map[string]revokedToken
	refreshTokens map[string]RefreshSession
	auditEvents   []AuditEvent
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
		refreshTokens: map[string]RefreshSession{},
		auditEvents:   []AuditEvent{},
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

	return r.updateUserRoleLocked(id, role)
}

func (r *MemoryRepository) UpdateUserRoleWithAudit(_ context.Context, id string, role identity.Role, event AuditEvent) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, err := r.updateUserRoleLocked(id, role)
	if err != nil {
		return User{}, err
	}
	if event.ID == "" {
		event.ID = NewID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	r.auditEvents = append(r.auditEvents, event)
	return user, nil
}

func (r *MemoryRepository) updateUserRoleLocked(id string, role identity.Role) (User, error) {
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

func (r *MemoryRepository) AuditEvents() []AuditEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]AuditEvent, len(r.auditEvents))
	copy(out, r.auditEvents)
	return out
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

func (r *MemoryRepository) CreateRefreshSession(_ context.Context, session RefreshSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshTokens[session.TokenHash] = session
	return nil
}

func (r *MemoryRepository) RotateRefreshSession(_ context.Context, oldHash, newHash string, newExpiresAt, now time.Time) (User, RefreshSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, exists := r.refreshTokens[oldHash]
	if !exists || !current.ExpiresAt.After(now) {
		return User{}, RefreshSession{}, ErrUnauthorized
	}
	user, exists := r.usersByID[current.UserID]
	if !exists {
		return User{}, RefreshSession{}, ErrUnauthorized
	}
	if current.RevokedAt != nil || current.ReplacedByHash != "" {
		r.revokeRefreshFamilyLocked(current.FamilyID, now)
		return User{}, RefreshSession{}, ErrRefreshReuse
	}

	current.RevokedAt = &now
	current.ReplacedByHash = newHash
	current.UpdatedAt = now
	r.refreshTokens[oldHash] = current

	next := RefreshSession{
		TokenHash: newHash,
		UserID:    current.UserID,
		FamilyID:  current.FamilyID,
		ExpiresAt: newExpiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.refreshTokens[newHash] = next
	return user, next, nil
}

func (r *MemoryRepository) RevokeRefreshSession(_ context.Context, tokenHash string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, exists := r.refreshTokens[tokenHash]
	if !exists {
		return nil
	}
	r.revokeRefreshFamilyLocked(current.FamilyID, now)
	return nil
}

func (r *MemoryRepository) revokeRefreshFamilyLocked(familyID string, now time.Time) {
	for hash, session := range r.refreshTokens {
		if session.FamilyID != familyID || session.RevokedAt != nil {
			continue
		}
		session.RevokedAt = &now
		session.UpdatedAt = now
		r.refreshTokens[hash] = session
	}
}
