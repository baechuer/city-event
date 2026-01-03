//go:build integration

package cases

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/bootstrap"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/messaging/rabbitmq"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/oauth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/redis"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/router"
	itinfra "github.com/baechuer/real-time-ressys/services/auth-service/test/integration/infra"
)

// Fake OAuth Provider for Integration Tests
type fakeGoogleProvider struct {
	authURL     string
	exchangeRes *oauth.TokenResponse
	userInfoRes *oauth.UserInfo
}

func (f *fakeGoogleProvider) IsConfigured() bool { return true }
func (f *fakeGoogleProvider) AuthURL(state, challenge string) string {
	return f.authURL + "&state=" + state
}
func (f *fakeGoogleProvider) ExchangeCode(ctx context.Context, code, verifier string) (*oauth.TokenResponse, error) {
	return f.exchangeRes, nil
}
func (f *fakeGoogleProvider) GetUserInfo(ctx context.Context, token string) (*oauth.UserInfo, error) {
	return f.userInfoRes, nil
}

func TestOAuthFlow(t *testing.T) {
	// 1. Setup Infra
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	// Open DB connection for verification
	db, err := sql.Open("pgx", env.PostgresDSN)
	require.NoError(t, err)
	defer db.Close()

	// Reset DB
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS oauth_identities, users CASCADE")

	if err := itinfra.EnsureAuthSchema(ctx, db); err != nil {
		t.Fatalf("EnsureAuthSchema failed: %v", err)
	}
	// Truncate tables
	_, err = db.Exec("TRUNCATE TABLE users, oauth_identities CASCADE")
	require.NoError(t, err)

	// Mock provider
	mockProvider := &fakeGoogleProvider{
		authURL: "https://accounts.google.com/o/oauth2/v2/auth?",
		exchangeRes: &oauth.TokenResponse{
			AccessToken: "mock-access-token",
			ExpiresIn:   3600,
		},
		userInfoRes: &oauth.UserInfo{
			Sub:           "123456789",
			Email:         "testuser@example.com",
			EmailVerified: true,
			Name:          "Test User",
		},
	}

	// 2. Build Server using Bootstrap
	deps := bootstrap.Deps{
		LoadConfig: func() (*config.Config, error) {
			return &config.Config{
				Env:                   "dev",
				HTTPAddr:              ":0", // random port
				JWTSecret:             "integration-test-secret",
				AccessTokenTTL:        15 * time.Minute,
				RefreshTokenTTL:       7 * 24 * time.Hour,
				VerifyEmailBaseURL:    "https://fe/verify?token=",
				PasswordResetBaseURL:  "https://fe/reset?token=",
				VerifyEmailTokenTTL:   24 * time.Hour,
				PasswordResetTokenTTL: 30 * time.Minute,

				DBAddr:        env.PostgresDSN,
				RedisAddr:     env.RedisAddr,
				RedisPassword: "",
				RedisDB:       0,
				RabbitURL:     env.RabbitURL,

				GoogleClientID:     "mock-client-id",
				GoogleClientSecret: "mock-client-secret",
				OAuthCallbackURL:   "http://localhost/callback",
				OAuthStateTTL:      10 * time.Minute,
				FrontendOrigin:     "http://localhost:3000",
				AllowedRedirects:   []string{"/dashboard"},

				DBDebug: true,
			}, nil
		},
		NewDB: func(addr string, debug bool) (bootstrap.DBCloser, error) {
			return sql.Open("pgx", addr)
		},
		NewRedis: func(addr, password string, db int) bootstrap.RedisClient {
			return redis.New(addr, password, db)
		},
		NewPublisher: func(url string) (bootstrap.Publisher, error) {
			return rabbitmq.NewPublisher(url)
		},
		NewRouter: func(d router.Deps) (http.Handler, error) {
			return router.New(d)
		},
		NewOAuthProvider: func() auth.OAuthProvider {
			return mockProvider
		},
	}

	srv, cleanup, err := bootstrap.NewServerWithDeps(deps)
	require.NoError(t, err)
	defer cleanup()

	// 3. Start Test
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	client := ts.Client() // follows redirects

	// 1. Start Flow
	t.Log("Step 1: Start OAuth Flow")

	// Needs no-redirect client to check 302
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := noRedirectClient.Get(ts.URL + "/auth/v1/oauth/google/start?redirect_to=/dashboard")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusFound, resp.StatusCode)
	loc := resp.Header.Get("Location")
	assert.Contains(t, loc, "https://accounts.google.com/o/oauth2/v2/auth?")
	assert.Contains(t, loc, "&state=")

	// Extract state from location
	startIdx := strings.Index(loc, "&state=") + 7
	stateToken := loc[startIdx:]
	if ampIdx := strings.Index(stateToken, "&"); ampIdx != -1 {
		stateToken = stateToken[:ampIdx]
	}

	t.Logf("Got state token: %s", stateToken)

	// 2. Callback Flow
	t.Log("Step 2: OAuth Callback")
	callbackURL := ts.URL + "/auth/v1/oauth/google/callback?code=mock-code&state=" + stateToken

	resp, err = client.Get(callbackURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Callback failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Check if cookies are set
	cookies := resp.Cookies()
	var refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			refreshCookie = c
			break
		}
	}
	require.NotNil(t, refreshCookie, "refresh_token cookie should be set")

	// Check DB side effects
	var count int
	err = db.QueryRow("SELECT count(*) FROM users WHERE email = $1", "testuser@example.com").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// 3. Security Tests
	t.Log("Step 3: Security Tests")

	// 3a. Disallowed Redirect
	resp, err = noRedirectClient.Get(ts.URL + "/auth/v1/oauth/google/start?redirect_to=https://evil.com")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusFound, resp.StatusCode)

	// 3b. Invalid State Replay
	replayCallbackURL := ts.URL + "/auth/v1/oauth/google/callback?code=new-code&state=" + stateToken
	resp, err = client.Get(replayCallbackURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid or expired oauth state")
}

func TestOAuthConflicts(t *testing.T) {
	// 1. Setup Infra
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	db, err := sql.Open("pgx", env.PostgresDSN)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// 2. Define Scenarios
	tests := []struct {
		name              string
		existingUserEmail string
		isVerified        bool
		expectedStatus    int
		expectedError     string
		expectLink        bool
	}{
		{
			name:              "Existing Verified User -> Auto Link",
			existingUserEmail: "verified@example.com",
			isVerified:        true,
			expectedStatus:    http.StatusOK, // Should succeed
			expectLink:        true,
		},
		{
			name:              "Existing Unverified User -> Error",
			existingUserEmail: "unverified@example.com",
			isVerified:        false,
			expectedStatus:    http.StatusBadRequest,
			expectedError:     "email already registered but not verified",
			expectLink:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Ensure Schema
			if err := itinfra.EnsureAuthSchema(ctx, db); err != nil {
				t.Fatalf("ensure schema failed: %v", err)
			}
			// Clean DB
			_, _ = db.ExecContext(ctx, "TRUNCATE TABLE users, oauth_identities CASCADE")

			// Create Existing User
			existingUserID := "existing-user-id"
			_, err = db.ExecContext(ctx, `
				INSERT INTO users (id, email, password_hash, role, email_verified, locked, created_at, updated_at)
				VALUES ($1, $2, 'hash', 'user', $3, false, now(), now())
			`, existingUserID, tc.existingUserEmail, tc.isVerified)
			require.NoError(t, err)

			// Mock Provider returning SAME email
			mockProvider := &fakeGoogleProvider{
				authURL: "https://accounts.google.com/o/oauth2/v2/auth?",
				exchangeRes: &oauth.TokenResponse{
					AccessToken: "mock-access-token",
					ExpiresIn:   3600,
				},
				userInfoRes: &oauth.UserInfo{
					Sub:           "google-sub-123",
					Email:         tc.existingUserEmail, // Same email
					EmailVerified: true,                 // Google says it's verified
					Name:          "Google User",
				},
			}

			// Bootstrap Server
			deps := bootstrap.Deps{
				LoadConfig: func() (*config.Config, error) {
					return &config.Config{
						Env:                "dev",
						HTTPAddr:           ":0",
						JWTSecret:          "test",
						AccessTokenTTL:     15 * time.Minute,
						RefreshTokenTTL:    7 * 24 * time.Hour,
						DBAddr:             env.PostgresDSN,
						RedisAddr:          env.RedisAddr,
						RabbitURL:          env.RabbitURL,
						GoogleClientID:     "mock",
						GoogleClientSecret: "mock",
						OAuthCallbackURL:   "http://localhost/callback",
						OAuthStateTTL:      10 * time.Minute,
						FrontendOrigin:     "http://localhost:3000",
						AllowedRedirects:   []string{"/"},
					}, nil
				},
				NewDB: func(addr string, debug bool) (bootstrap.DBCloser, error) {
					return sql.Open("pgx", addr)
				},
				NewRedis: func(addr, password string, db int) bootstrap.RedisClient {
					return redis.New(addr, password, db)
				},
				NewPublisher: func(url string) (bootstrap.Publisher, error) {
					return rabbitmq.NewPublisher(url)
				},
				NewRouter: func(d router.Deps) (http.Handler, error) {
					return router.New(d)
				},
				NewOAuthProvider: func() auth.OAuthProvider {
					return mockProvider
				},
			}

			srv, cleanup, err := bootstrap.NewServerWithDeps(deps)
			require.NoError(t, err)
			defer cleanup()

			ts := httptest.NewServer(srv.Handler)
			defer ts.Close()
			client := ts.Client() // Default client (follows redirects) for callback

			// Custom client that DOES NOT follow redirects for Start flow
			noRedirectClient := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			// 1. Start Flow to get State
			resp, err := noRedirectClient.Get(ts.URL + "/auth/v1/oauth/google/start?redirect_to=/")
			require.NoError(t, err)

			if resp.StatusCode != http.StatusFound {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("Expected 302, got %d. Body: %q", resp.StatusCode, string(body))
			}
			require.Equal(t, http.StatusFound, resp.StatusCode, "Expected redirect from OAuthStart")

			loc := resp.Header.Get("Location")
			require.Contains(t, loc, "&state=", "Location header should contain state")

			startIdx := strings.Index(loc, "&state=") + 7
			stateToken := loc[startIdx:]
			if ampIdx := strings.Index(stateToken, "&"); ampIdx != -1 {
				stateToken = stateToken[:ampIdx]
			}

			// 2. Callback
			callbackURL := ts.URL + "/auth/v1/oauth/google/callback?code=code&state=" + stateToken
			resp, err = client.Get(callbackURL)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectedStatus == http.StatusOK {
				// Verify Linked
				if tc.expectLink {
					var count int
					err = db.QueryRow("SELECT count(*) FROM oauth_identities WHERE user_id = $1 AND provider = 'google'", existingUserID).Scan(&count)
					require.NoError(t, err)
					assert.Equal(t, 1, count, "Should have linked oauth identity to existing user")
				}
			} else {
				// Verify Error Message
				body, _ := io.ReadAll(resp.Body)
				assert.Contains(t, string(body), tc.expectedError)
			}
		})
	}
}
