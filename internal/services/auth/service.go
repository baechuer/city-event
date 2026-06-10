package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrTooManyAttempts    = errors.New("too many attempts")
)

type Service struct {
	repo            Repository
	hasher          PasswordHasher
	tokens          TokenManager
	refreshTokenTTL time.Duration
	guard           *LoginGuard
	now             func() time.Time
}

type AuthResult struct {
	User             PublicUser `json:"user"`
	AccessToken      string     `json:"accessToken"`
	RefreshToken     string     `json:"-"`
	RefreshExpiresAt time.Time  `json:"-"`
}

func NewService(repo Repository, hasher PasswordHasher, tokens TokenManager, guard *LoginGuard) *Service {
	if guard == nil {
		guard = NewLoginGuard(5, time.Minute)
	}
	return &Service{
		repo:            repo,
		hasher:          hasher,
		tokens:          tokens,
		refreshTokenTTL: 30 * 24 * time.Hour,
		guard:           guard,
		now:             time.Now,
	}
}

func NewDefaultService(repo Repository, jwtSecret, jwtIssuer string, accessTokenTTL time.Duration) *Service {
	return NewDefaultServiceWithRefreshTTL(repo, jwtSecret, jwtIssuer, accessTokenTTL, 30*24*time.Hour)
}

func NewDefaultServiceWithRefreshTTL(repo Repository, jwtSecret, jwtIssuer string, accessTokenTTL, refreshTokenTTL time.Duration) *Service {
	svc := NewService(
		repo,
		NewPasswordHasher(bcrypt.DefaultCost),
		NewTokenManager(jwtSecret, jwtIssuer, accessTokenTTL),
		NewLoginGuard(5, time.Minute),
	)
	svc.refreshTokenTTL = refreshTokenTTL
	return svc
}

func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (AuthResult, error) {
	email := NormalizeEmail(cmd.Email)
	displayName := strings.TrimSpace(cmd.DisplayName)
	if err := ValidateEmail(email); err != nil {
		return AuthResult{}, err
	}
	if err := ValidatePassword(cmd.Password); err != nil {
		return AuthResult{}, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return AuthResult{}, err
	}

	hash, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return AuthResult{}, err
	}
	role := identity.NormalizeRole(string(cmd.Role))
	user, err := NewUser(email, displayName, role, hash, s.now())
	if err != nil {
		return AuthResult{}, err
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, user)
}

func (s *Service) EnsureSeedAdmin(ctx context.Context, cmd SeedAdminCommand) (PublicUser, error) {
	email := NormalizeEmail(cmd.Email)
	displayName := strings.TrimSpace(cmd.DisplayName)
	if displayName == "" {
		displayName = "CityEvents Admin"
	}
	if err := ValidateEmail(email); err != nil {
		return PublicUser{}, err
	}
	if err := ValidatePassword(cmd.Password); err != nil {
		return PublicUser{}, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return PublicUser{}, err
	}

	existing, err := s.repo.FindUserByEmail(ctx, email)
	if err == nil {
		if existing.Role != identity.RoleAdmin {
			existing, err = s.repo.UpdateUserRole(ctx, existing.ID, identity.RoleAdmin)
			if err != nil {
				return PublicUser{}, err
			}
		}
		return existing.Public(), nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return PublicUser{}, err
	}

	hash, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return PublicUser{}, err
	}
	admin, err := NewUser(email, displayName, identity.RoleAdmin, hash, s.now())
	if err != nil {
		return PublicUser{}, err
	}
	if err := s.repo.CreateUser(ctx, admin); err != nil {
		return PublicUser{}, err
	}
	return admin.Public(), nil
}

func (s *Service) UpdateUserRole(ctx context.Context, cmd UpdateRoleCommand) (PublicUser, error) {
	if strings.TrimSpace(cmd.ActorUserID) == "" || strings.TrimSpace(cmd.TargetUserID) == "" {
		return PublicUser{}, ErrUnauthorized
	}
	role := identity.NormalizeRole(string(cmd.Role))
	if err := ValidateRole(role); err != nil {
		return PublicUser{}, err
	}
	actor, err := s.repo.FindUserByID(ctx, strings.TrimSpace(cmd.ActorUserID))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return PublicUser{}, ErrUnauthorized
		}
		return PublicUser{}, err
	}
	if !identity.CanAdmin(actor.Role) {
		return PublicUser{}, ErrForbidden
	}
	updated, err := s.repo.UpdateUserRole(ctx, strings.TrimSpace(cmd.TargetUserID), role)
	if err != nil {
		return PublicUser{}, err
	}
	return updated.Public(), nil
}

func (s *Service) Login(ctx context.Context, cmd LoginCommand) (AuthResult, error) {
	email := NormalizeEmail(cmd.Email)
	if !s.guard.Allow(email) {
		return AuthResult{}, ErrTooManyAttempts
	}

	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		s.guard.RecordFailure(email)
		if errors.Is(err, ErrUserNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}

	if !s.hasher.Verify(user.PasswordHash, cmd.Password) {
		s.guard.RecordFailure(email)
		return AuthResult{}, ErrInvalidCredentials
	}

	s.guard.Reset(email)
	return s.issue(ctx, user)
}

func (s *Service) CurrentUser(ctx context.Context, accessToken string) (PublicUser, error) {
	claims, err := s.tokens.Verify(accessToken)
	if err != nil {
		return PublicUser{}, ErrUnauthorized
	}
	revoked, err := s.repo.IsTokenRevoked(ctx, claims.TokenID)
	if err != nil {
		return PublicUser{}, err
	}
	if revoked {
		return PublicUser{}, ErrUnauthorized
	}

	user, err := s.repo.FindUserByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return PublicUser{}, ErrUnauthorized
		}
		return PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return AuthResult{}, ErrUnauthorized
	}
	now := s.now().UTC()
	nextRaw, nextHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	user, session, err := s.repo.RotateRefreshSession(ctx, HashRefreshToken(refreshToken), nextHash, now.Add(s.refreshTokenTTL), now)
	if err != nil {
		if errors.Is(err, ErrRefreshReuse) {
			return AuthResult{}, ErrUnauthorized
		}
		return AuthResult{}, err
	}
	accessToken, _, err := s.tokens.Sign(user)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		User:             user.Public(),
		AccessToken:      accessToken,
		RefreshToken:     nextRaw,
		RefreshExpiresAt: session.ExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, accessToken, refreshToken string) error {
	if strings.TrimSpace(accessToken) != "" {
		claims, err := s.tokens.Verify(accessToken)
		if err != nil {
			return ErrUnauthorized
		}
		if err := s.repo.RevokeToken(ctx, claims.TokenID, claims.UserID, claims.ExpiresAt); err != nil {
			return err
		}
	}
	return s.LogoutRefresh(ctx, refreshToken)
}

func (s *Service) LogoutRefresh(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil
	}
	return s.repo.RevokeRefreshSession(ctx, HashRefreshToken(refreshToken), s.now().UTC())
}

func (s *Service) issue(ctx context.Context, user User) (AuthResult, error) {
	token, _, err := s.tokens.Sign(user)
	if err != nil {
		return AuthResult{}, err
	}
	refreshToken, refreshHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	now := s.now().UTC()
	session := RefreshSession{
		TokenHash: refreshHash,
		UserID:    user.ID,
		FamilyID:  NewID(),
		ExpiresAt: now.Add(s.refreshTokenTTL),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateRefreshSession(ctx, session); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		User:             user.Public(),
		AccessToken:      token,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: session.ExpiresAt,
	}, nil
}

type LoginGuard struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	now      func() time.Time
	attempts map[string]loginAttempt
}

type loginAttempt struct {
	Count     int
	FirstSeen time.Time
}

func NewLoginGuard(max int, window time.Duration) *LoginGuard {
	return &LoginGuard{
		max:      max,
		window:   window,
		now:      time.Now,
		attempts: map[string]loginAttempt{},
	}
}

func (g *LoginGuard) Allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	key = NormalizeEmail(key)
	attempt := g.attempts[key]
	if attempt.Count == 0 {
		return true
	}
	if g.now().Sub(attempt.FirstSeen) > g.window {
		delete(g.attempts, key)
		return true
	}
	return attempt.Count < g.max
}

func (g *LoginGuard) RecordFailure(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	key = NormalizeEmail(key)
	now := g.now()
	attempt := g.attempts[key]
	if attempt.Count == 0 || now.Sub(attempt.FirstSeen) > g.window {
		g.attempts[key] = loginAttempt{Count: 1, FirstSeen: now}
		return
	}
	attempt.Count++
	g.attempts[key] = attempt
}

func (g *LoginGuard) Reset(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, NormalizeEmail(key))
}
