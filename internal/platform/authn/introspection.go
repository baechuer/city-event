package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/baechuer/cityevents/internal/platform/observability"
)

var ErrIntrospectionUnavailable = errors.New("auth introspection unavailable")

type Introspector interface {
	Introspect(ctx context.Context, accessToken string) (Subject, error)
}

type HTTPIntrospector struct {
	baseURL *url.URL
	client  *http.Client
}

func NewHTTPIntrospector(authServiceURL string, transport http.RoundTripper) (*HTTPIntrospector, error) {
	parsed, err := url.Parse(strings.TrimSpace(authServiceURL))
	if err != nil {
		return nil, fmt.Errorf("parse auth service url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("auth service url must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, fmt.Errorf("auth service url must include host")
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &HTTPIntrospector{
		baseURL: parsed,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		},
	}, nil
}

func (i *HTTPIntrospector) Introspect(ctx context.Context, accessToken string) (Subject, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return Subject{}, ErrInvalidToken
	}
	meURL := i.baseURL.ResolveReference(&url.URL{Path: "/v1/auth/me"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meURL.String(), nil)
	if err != nil {
		return Subject{}, fmt.Errorf("%w: %v", ErrIntrospectionUnavailable, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	observability.InjectTraceHeaders(req.Header, ctx)

	resp, err := i.client.Do(req)
	if err != nil {
		return Subject{}, fmt.Errorf("%w: %v", ErrIntrospectionUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Subject{}, ErrInvalidToken
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Subject{}, fmt.Errorf("%w: status %d", ErrIntrospectionUnavailable, resp.StatusCode)
	}

	var payload struct {
		User struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	limited := io.LimitReader(resp.Body, 32*1024)
	if err := json.NewDecoder(limited).Decode(&payload); err != nil {
		return Subject{}, fmt.Errorf("%w: decode response: %v", ErrIntrospectionUnavailable, err)
	}

	role := identity.NormalizeRole(payload.User.Role)
	subject := Subject{
		UserID: strings.TrimSpace(payload.User.ID),
		Email:  strings.TrimSpace(payload.User.Email),
		Role:   role,
	}
	if subject.UserID == "" || !identity.ValidRole(role) {
		return Subject{}, ErrInvalidToken
	}
	return subject, nil
}
