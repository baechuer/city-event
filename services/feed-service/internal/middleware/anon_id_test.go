package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAnonID_NewCookie(t *testing.T) {
	secret := "test-secret"
	ttl := 24 * time.Hour

	handler := AnonID(secret, ttl, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anonID, ok := AnonIDFromContext(r.Context())
		if !ok || anonID == "" {
			t.Error("expected anon_id in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Check cookie is set
	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "anon_id" {
			found = true
			if c.Value == "" {
				t.Error("cookie value should not be empty")
			}
		}
	}
	if !found {
		t.Error("expected anon_id cookie to be set")
	}
}

func TestAnonID_ExistingCookie(t *testing.T) {
	secret := "test-secret"
	ttl := 24 * time.Hour
	exp := time.Now().Add(ttl).Unix()
	signedCookie := signAnonCookie(secret, "test-anon-id", exp)

	handler := AnonID(secret, ttl, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anonID, ok := AnonIDFromContext(r.Context())
		if !ok {
			t.Error("expected anon_id in context")
		}
		if anonID != "test-anon-id" {
			t.Errorf("expected 'test-anon-id', got '%s'", anonID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "anon_id", Value: signedCookie})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAnonID_ExpiredCookie(t *testing.T) {
	secret := "test-secret"
	ttl := 24 * time.Hour
	exp := time.Now().Add(-1 * time.Hour).Unix() // Expired
	signedCookie := signAnonCookie(secret, "old-anon-id", exp)

	var gotAnonID string
	handler := AnonID(secret, ttl, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAnonID, _ = AnonIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "anon_id", Value: signedCookie})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should get a new anon_id, not the expired one
	if gotAnonID == "old-anon-id" {
		t.Error("should not use expired anon_id")
	}
	if gotAnonID == "" {
		t.Error("should generate new anon_id")
	}
}

func TestAnonID_TamperedCookie(t *testing.T) {
	secret := "test-secret"
	ttl := 24 * time.Hour

	var gotAnonID string
	handler := AnonID(secret, ttl, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAnonID, _ = AnonIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Tampered cookie (wrong signature)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "anon_id", Value: "tampered.12345.invalid"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should get a new anon_id
	if gotAnonID == "tampered" {
		t.Error("should not accept tampered cookie")
	}
}
