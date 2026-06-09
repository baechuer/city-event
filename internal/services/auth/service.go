package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrTooManyAttempts    = errors.New("too many attempts")
)

type Service struct {
	repo   Repository
	hasher PasswordHasher
	tokens TokenManager
	guard  *LoginGuard
	now    func() time.Time
}

type AuthResult struct {
	User        PublicUser `json:"user"`
	AccessToken string     `json:"accessToken"`
}

func NewService(repo Repository, hasher PasswordHasher, tokens TokenManager, guard *LoginGuard) *Service {
	if guard == nil {
		guard = NewLoginGuard(5, time.Minute)
	}
	return &Service{
		repo:   repo,
		hasher: hasher,
		tokens: tokens,
		guard:  guard,
		now:    time.Now,
	}
}

func NewDefaultService(repo Repository, jwtSecret, jwtIssuer string, accessTokenTTL time.Duration) *Service {
	return NewService(
		repo,
		NewPasswordHasher(bcrypt.DefaultCost),
		NewTokenManager(jwtSecret, jwtIssuer, accessTokenTTL),
		NewLoginGuard(5, time.Minute),
	)
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
	user, err := NewUser(email, displayName, hash, s.now())
	if err != nil {
		return AuthResult{}, err
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return AuthResult{}, err
	}
	return s.issue(user)
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
	return s.issue(user)
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

func (s *Service) Logout(ctx context.Context, accessToken string) error {
	claims, err := s.tokens.Verify(accessToken)
	if err != nil {
		return ErrUnauthorized
	}
	return s.repo.RevokeToken(ctx, claims.TokenID, claims.UserID, claims.ExpiresAt)
}

func (s *Service) issue(user User) (AuthResult, error) {
	token, _, err := s.tokens.Sign(user)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: user.Public(), AccessToken: token}, nil
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
