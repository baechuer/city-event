package authn

import (
	"errors"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestTokenManagerVerifyRejectsUnexpectedHeader(t *testing.T) {
	manager := NewTokenManager("test-secret", "cityevents", time.Hour)
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	manager.Now = func() time.Time { return now }

	token := signedTestToken(t, manager, map[string]string{"alg": "none", "typ": "JWT"}, now)
	if _, err := manager.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("verify error = %v, want ErrInvalidToken", err)
	}
}

func TestTokenManagerVerifyRejectsMissingTypeHeader(t *testing.T) {
	manager := NewTokenManager("test-secret", "cityevents", time.Hour)
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	manager.Now = func() time.Time { return now }

	token := signedTestToken(t, manager, map[string]string{"alg": "HS256"}, now)
	if _, err := manager.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("verify error = %v, want ErrInvalidToken", err)
	}
}

func signedTestToken(t *testing.T, manager TokenManager, header map[string]string, now time.Time) string {
	t.Helper()
	headerPart, err := encodeJSON(header)
	if err != nil {
		t.Fatalf("encode header: %v", err)
	}
	payloadPart, err := encodeJSON(map[string]any{
		"sub":   "user-1",
		"email": "user-1@example.com",
		"role":  identity.RoleUser,
		"jti":   "token-1",
		"iss":   manager.Issuer,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	unsigned := headerPart + "." + payloadPart
	return unsigned + "." + manager.sign(unsigned)
}
