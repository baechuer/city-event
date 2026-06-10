package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type TokenManager struct {
	Secret []byte
	Issuer string
	TTL    time.Duration
	Now    func() time.Time
}

type Claims struct {
	UserID    string
	Email     string
	Role      identity.Role
	TokenID   string
	Issuer    string
	IssuedAt  time.Time
	ExpiresAt time.Time
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
	now := m.now().UTC()
	claims := Claims{
		UserID:    user.ID,
		Email:     user.Email,
		Role:      identity.NormalizeRole(string(user.Role)),
		TokenID:   NewID(),
		Issuer:    m.Issuer,
		IssuedAt:  now,
		ExpiresAt: now.Add(m.TTL),
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	payload := map[string]any{
		"sub":   claims.UserID,
		"email": claims.Email,
		"role":  claims.Role,
		"jti":   claims.TokenID,
		"iss":   claims.Issuer,
		"iat":   claims.IssuedAt.Unix(),
		"exp":   claims.ExpiresAt.Unix(),
	}

	headerPart, err := encodeJSON(header)
	if err != nil {
		return "", Claims{}, err
	}
	payloadPart, err := encodeJSON(payload)
	if err != nil {
		return "", Claims{}, err
	}

	unsigned := headerPart + "." + payloadPart
	signature := m.sign(unsigned)
	return unsigned + "." + signature, claims, nil
}

func (m TokenManager) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	expected := m.sign(unsigned)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, ErrInvalidToken
	}

	var payload struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
		Role    string `json:"role"`
		TokenID string `json:"jti"`
		Issuer  string `json:"iss"`
		Issued  int64  `json:"iat"`
		Expires int64  `json:"exp"`
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if payload.Issuer != m.Issuer || payload.Subject == "" || payload.TokenID == "" {
		return Claims{}, ErrInvalidToken
	}
	role := identity.NormalizeRole(payload.Role)
	if !identity.ValidRole(role) {
		return Claims{}, ErrInvalidToken
	}

	claims := Claims{
		UserID:    payload.Subject,
		Email:     payload.Email,
		Role:      role,
		TokenID:   payload.TokenID,
		Issuer:    payload.Issuer,
		IssuedAt:  time.Unix(payload.Issued, 0).UTC(),
		ExpiresAt: time.Unix(payload.Expires, 0).UTC(),
	}
	if !claims.ExpiresAt.After(m.now()) {
		return Claims{}, ErrExpiredToken
	}
	return claims, nil
}

func (m TokenManager) sign(unsigned string) string {
	mac := hmac.New(sha256.New, m.Secret)
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m TokenManager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func encodeJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal jwt part: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
