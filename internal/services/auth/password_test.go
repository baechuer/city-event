package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHasher(t *testing.T) {
	hasher := NewPasswordHasher(bcrypt.MinCost)

	hash, err := hasher.Hash("StrongerPass123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "StrongerPass123" {
		t.Fatalf("hash must not equal plaintext")
	}
	if !hasher.Verify(hash, "StrongerPass123") {
		t.Fatalf("expected correct password to verify")
	}
	if hasher.Verify(hash, "WrongPass123") {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestPasswordHasherRejectsWeakPassword(t *testing.T) {
	hasher := NewPasswordHasher(bcrypt.MinCost)
	if _, err := hasher.Hash("weak"); err == nil {
		t.Fatalf("expected weak password to be rejected")
	}
}
