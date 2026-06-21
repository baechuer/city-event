package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestHTTPEventAuthorizerAllowsOrganizerOwnerAndAdmin(t *testing.T) {
	seenAuthorization := ""
	eventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuthorization = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/events/event-1" {
			t.Fatalf("path = %s, want /v1/events/event-1", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event": map[string]string{
				"id":          "event-1",
				"organizerId": "organizer-1",
			},
		})
	}))
	defer eventServer.Close()

	authorizer, err := NewHTTPEventAuthorizer(eventServer.URL, nil)
	if err != nil {
		t.Fatalf("new authorizer: %v", err)
	}

	err = authorizer.AuthorizeCreateMedia(context.Background(), EventMediaAuthorization{
		EventID:     "event-1",
		UserID:      "organizer-1",
		Role:        identity.RoleOrganizer,
		AccessToken: "access-token",
	})
	if err != nil {
		t.Fatalf("authorize organizer: %v", err)
	}
	if seenAuthorization != "Bearer access-token" {
		t.Fatalf("authorization header = %q", seenAuthorization)
	}

	err = authorizer.AuthorizeCreateMedia(context.Background(), EventMediaAuthorization{
		EventID: "event-1",
		UserID:  "admin-1",
		Role:    identity.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("authorize admin: %v", err)
	}
}

func TestHTTPEventAuthorizerRejectsNonOwnerAndPlainUser(t *testing.T) {
	eventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event": map[string]string{
				"id":          "event-1",
				"organizerId": "organizer-1",
			},
		})
	}))
	defer eventServer.Close()

	authorizer, err := NewHTTPEventAuthorizer(eventServer.URL, nil)
	if err != nil {
		t.Fatalf("new authorizer: %v", err)
	}

	for name, request := range map[string]EventMediaAuthorization{
		"plain user": {
			EventID: "event-1",
			UserID:  "user-1",
			Role:    identity.RoleUser,
		},
		"other organizer": {
			EventID: "event-1",
			UserID:  "organizer-2",
			Role:    identity.RoleOrganizer,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := authorizer.AuthorizeCreateMedia(context.Background(), request); !errors.Is(err, ErrForbidden) {
				t.Fatalf("authorize error = %v, want ErrForbidden", err)
			}
		})
	}
}

func TestHTTPEventAuthorizerReturnsNotFoundForMissingEvent(t *testing.T) {
	eventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer eventServer.Close()

	authorizer, err := NewHTTPEventAuthorizer(eventServer.URL, nil)
	if err != nil {
		t.Fatalf("new authorizer: %v", err)
	}
	err = authorizer.AuthorizeCreateMedia(context.Background(), EventMediaAuthorization{
		EventID: "missing",
		UserID:  "admin-1",
		Role:    identity.RoleAdmin,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("authorize missing error = %v, want ErrNotFound", err)
	}
}
