package auth

import (
	"errors"
	"testing"
	"time"
)

func TestTokenManagerSignsAndVerifies(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	manager := NewTokenManager("secret", "cityevents", time.Hour)
	manager.Now = func() time.Time { return now }

	user := User{ID: "user-1", Email: "user@example.com"}
	token, signedClaims, err := manager.Sign(user)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	claims, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.UserID != user.ID || claims.Email != user.Email || claims.TokenID == "" {
		t.Fatalf("unexpected claims: %+v signed=%+v", claims, signedClaims)
	}
}

func TestTokenManagerRejectsWrongSecret(t *testing.T) {
	manager := NewTokenManager("secret", "cityevents", time.Hour)
	token, _, err := manager.Sign(User{ID: "user-1", Email: "user@example.com"})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	other := NewTokenManager("other-secret", "cityevents", time.Hour)
	if _, err := other.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
}

func TestTokenManagerRejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	manager := NewTokenManager("secret", "cityevents", time.Hour)
	manager.Now = func() time.Time { return now }
	token, _, err := manager.Sign(User{ID: "user-1", Email: "user@example.com"})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	manager.Now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := manager.Verify(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected expired token, got %v", err)
	}
}
