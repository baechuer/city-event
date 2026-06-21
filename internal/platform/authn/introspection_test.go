package authn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestHTTPIntrospectorReturnsCurrentSubject(t *testing.T) {
	seenAuthorization := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/me" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		seenAuthorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]string{
				"id":    "user-1",
				"email": "user-1@example.com",
				"role":  string(identity.RoleAdmin),
			},
		})
	}))
	defer server.Close()

	introspector, err := NewHTTPIntrospector(server.URL, nil)
	if err != nil {
		t.Fatalf("new introspector: %v", err)
	}
	subject, err := introspector.Introspect(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if seenAuthorization != "Bearer access-token" {
		t.Fatalf("authorization = %q", seenAuthorization)
	}
	if subject.UserID != "user-1" || subject.Email != "user-1@example.com" || subject.Role != identity.RoleAdmin {
		t.Fatalf("subject = %+v", subject)
	}
}

func TestHTTPIntrospectorRejectsInvalidOrUnavailableResponses(t *testing.T) {
	tests := map[string]struct {
		status int
		body   string
		want   error
	}{
		"unauthorized": {
			status: http.StatusUnauthorized,
			body:   `{"error":{"code":"unauthorized"}}`,
			want:   ErrInvalidToken,
		},
		"bad gateway dependency": {
			status: http.StatusBadGateway,
			body:   `{"error":{"code":"bad_gateway"}}`,
			want:   ErrIntrospectionUnavailable,
		},
		"malformed payload": {
			status: http.StatusOK,
			body:   `{"user":{"id":"","role":"ADMIN"}}`,
			want:   ErrInvalidToken,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			introspector, err := NewHTTPIntrospector(server.URL, nil)
			if err != nil {
				t.Fatalf("new introspector: %v", err)
			}
			_, err = introspector.Introspect(context.Background(), "access-token")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewHTTPIntrospectorRejectsInvalidServiceURL(t *testing.T) {
	for _, rawURL := range []string{"", "ftp://auth-service", "http:///missing-host"} {
		if _, err := NewHTTPIntrospector(rawURL, nil); err == nil {
			t.Fatalf("expected invalid URL %q to fail", rawURL)
		}
	}
}
