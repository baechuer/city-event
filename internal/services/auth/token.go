package auth

import (
	"time"

	"github.com/baechuer/cityevents/internal/platform/authn"
)

var (
	ErrInvalidToken = authn.ErrInvalidToken
	ErrExpiredToken = authn.ErrExpiredToken
)

type Claims = authn.Claims

type TokenManager struct {
	Secret []byte
	Issuer string
	TTL    time.Duration
	Now    func() time.Time
}

func NewTokenManager(secret, issuer string, ttl time.Duration) TokenManager {
	return TokenManager{
		Secret: []byte(secret),
		Issuer: issuer,
		TTL:    ttl,
		Now:    time.Now,
	}
}

func (m TokenManager) Sign(user User) (string, Claims, error) {
	return m.manager().Sign(authn.Subject{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
	})
}

func (m TokenManager) Verify(token string) (Claims, error) {
	return m.manager().Verify(token)
}

func (m TokenManager) manager() authn.TokenManager {
	return authn.TokenManager{
		Secret: m.Secret,
		Issuer: m.Issuer,
		TTL:    m.TTL,
		Now:    m.Now,
	}
}
