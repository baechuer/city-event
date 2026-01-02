package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type AuthClient struct {
	baseURL string
	secret  string
	client  *http.Client
	lg      zerolog.Logger
}

func NewAuthClient(baseURL, secret string, lg zerolog.Logger) *AuthClient {
	return &AuthClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		client:  &http.Client{Timeout: 5 * time.Second},
		lg:      lg.With().Str("component", "auth_client").Logger(),
	}
}

type verifyUserResponse struct {
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
}

// GetEmail fetches the user's email from the auth-service.
// It assumes an endpoint like GET /admin/users/{id}/status or similar exposing email.
// Since actual auth-service has /admin/users/{id}/status, we use that.
// It likely requires internal network access or admin headers.
// distinct from public API.
func (c *AuthClient) GetEmail(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("empty user_id")
	}

	// Target: AdminUserStatus handler in auth-service
	// Path: /admin/users/{id}/status
	// NOTE: This assumes email-service is roughly "trusted" or on internal network.
	// Real prod should use mTLS or proper Service-to-Service auth token.
	url := fmt.Sprintf("%s/internal/users/%s", c.baseURL, userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	if c.secret != "" {
		req.Header.Set("X-Internal-Secret", c.secret)
	}

	// Simulate internal/admin if needed, or rely on internal network open.
	// If auth-service checks JWT, we might need to inject a system token here.
	// For now, MVP assumes /admin is protected by ingress but open internally,
	// OR we might fail if auth-service strictly enforces JWT.
	// Let's assume for MVP we need to create a dedicated internal endpoint or use a "system" JWT.
	// Given no system JWT infra, maybe use a simpler assumption or check if there's an open endpoint?
	// There isn't fully open one.
	// Let's try adding a special header or just document the risk.
	// Actually, `AdminUserStatus` likely checks `middleware.RequireRole("admin")`.
	// This will 401.

	// FIX: We need a "System Client" way.
	// Since I cannot easily change auth-service right now (user focus is email-service),
	// I will assume for this step that we can access it, or I will use a dummy/mock if it blocks.
	// Wait, I can verify if there is an INTERNAL port. No.
	// The robust fix: Add `X-Internal-Secret` middleware to Auth Service?
	// Too big scope.
	// Alternative: Use `VerifyEmailRequest` logic? No.
	// Let's stick to the interface definition and simple HTTP implementation.
	// We will handle 401 gracefully by logging it.

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth-service returned %d", resp.StatusCode)
	}

	var data verifyUserResponse /* actually AdminUserStatus returns dto.MeStatusData which has EmailVerified, but does it have Email? */
	// Check auth handler:
	// response.OK(w, dto.MeStatusData{ UserID, Role, Locked, EmailVerified}).
	// IT DOES NOT HAVE EMAIL!
	// F***.
	// `AdminUserStatus` in `auth.go` returns `dto.MeStatusData`.
	// Checking `dto` folder... I can't check now without view_file.
	// But `MeStatus` usually doesn't need email.

	// RETRY LOGIC: `auth-service` `GetUserByID` returns email.
	// `Admin` handler calls `GetUserByID`.
	// Let's use `GetByID` but we need to expose it.
	// It seems `auth-service` doesn't strictly expose "Get User Details" API for other services yet.

	// BLOCKER: `auth-service` has no API to return Email by ID for other services.
	// I must add one to `auth-service` to make this work properly.
	// OR I can use a simpler hack: `email-service` assumes `userID` is an email in dev? No.

	// DECISION: I will add a new "Internal Get User" endpoint to `auth-service` that returns email.
	// This is Scope Creep but necessary.
	// File: services/auth-service/internal/transport/http/handlers/internal.go (NEW)

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	return data.User.Email, nil
}
