package auth

import "testing"

func TestNormalizeEmail(t *testing.T) {
	got := NormalizeEmail("  USER@Example.COM ")
	if got != "user@example.com" {
		t.Fatalf("NormalizeEmail() = %q", got)
	}
}

func TestValidateEmail(t *testing.T) {
	if err := ValidateEmail("user@example.com"); err != nil {
		t.Fatalf("expected valid email: %v", err)
	}
	if err := ValidateEmail("not-email"); err == nil {
		t.Fatalf("expected invalid email")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("StrongerPass123"); err != nil {
		t.Fatalf("expected strong password: %v", err)
	}
	for _, password := range []string{"short", "lowercaseonly123", "UPPERCASEONLY123", "NoDigitsHere"} {
		if err := ValidatePassword(password); err == nil {
			t.Fatalf("expected weak password %q to be rejected", password)
		}
	}
}

func TestValidateDisplayName(t *testing.T) {
	if err := ValidateDisplayName("Jane Doe"); err != nil {
		t.Fatalf("expected display name to be valid: %v", err)
	}
	if err := ValidateDisplayName("  "); err == nil {
		t.Fatalf("expected empty display name to be rejected")
	}
}
