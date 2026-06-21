package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/baechuer/cityevents/internal/platform/observability"
)

type EventAuthorizer interface {
	AuthorizeCreateMedia(context.Context, EventMediaAuthorization) error
}

type EventMediaAuthorization struct {
	EventID     string
	UserID      string
	Role        identity.Role
	AccessToken string
}

type HTTPEventAuthorizer struct {
	baseURL *url.URL
	client  *http.Client
}

func NewHTTPEventAuthorizer(eventServiceURL string, transport http.RoundTripper) (*HTTPEventAuthorizer, error) {
	baseURL, err := url.Parse(strings.TrimSpace(eventServiceURL))
	if err != nil {
		return nil, err
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("event service url scheme must be http or https")
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("event service url host is required")
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &HTTPEventAuthorizer{
		baseURL: baseURL,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		},
	}, nil
}

func (a *HTTPEventAuthorizer) AuthorizeCreateMedia(ctx context.Context, request EventMediaAuthorization) error {
	eventID := strings.TrimSpace(request.EventID)
	userID := strings.TrimSpace(request.UserID)
	role := identity.NormalizeRole(string(request.Role))
	if eventID == "" {
		return ErrInvalidUpload
	}
	if userID == "" {
		return ErrUnauthorized
	}
	if !identity.CanPublishEvents(role) {
		return ErrForbidden
	}

	event, err := a.fetchEvent(ctx, eventID, request.AccessToken)
	if err != nil {
		return err
	}
	if identity.CanAdmin(role) {
		return nil
	}
	if event.OrganizerID != userID {
		return ErrForbidden
	}
	return nil
}

type eventAuthorizationDetail struct {
	ID          string
	OrganizerID string
}

func (a *HTTPEventAuthorizer) fetchEvent(ctx context.Context, eventID, accessToken string) (eventAuthorizationDetail, error) {
	eventURL := a.baseURL.ResolveReference(&url.URL{Path: "/v1/events/" + url.PathEscape(eventID)})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eventURL.String(), nil)
	if err != nil {
		return eventAuthorizationDetail{}, err
	}
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	observability.InjectTraceHeaders(req.Header, ctx)

	resp, err := a.client.Do(req)
	if err != nil {
		return eventAuthorizationDetail{}, fmt.Errorf("event authorization lookup failed: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return eventAuthorizationDetail{}, ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return eventAuthorizationDetail{}, ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return eventAuthorizationDetail{}, fmt.Errorf("event authorization lookup returned status %d", resp.StatusCode)
	}

	var payload struct {
		Event struct {
			ID          string `json:"id"`
			OrganizerID string `json:"organizerId"`
		} `json:"event"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return eventAuthorizationDetail{}, fmt.Errorf("decode event authorization response: %w", err)
	}
	if strings.TrimSpace(payload.Event.ID) == "" || strings.TrimSpace(payload.Event.OrganizerID) == "" {
		return eventAuthorizationDetail{}, ErrNotFound
	}
	return eventAuthorizationDetail{ID: payload.Event.ID, OrganizerID: strings.TrimSpace(payload.Event.OrganizerID)}, nil
}

type denyEventAuthorizer struct{}

func (denyEventAuthorizer) AuthorizeCreateMedia(context.Context, EventMediaAuthorization) error {
	return ErrForbidden
}
