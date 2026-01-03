package auth

import (
	"context"
	"fmt"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/oauth"
	"github.com/google/uuid"
)

// OAuthProvider defines the methods required from an OAuth provider (like Google)
type OAuthProvider interface {
	IsConfigured() bool
	AuthURL(state, codeChallenge string) string
	ExchangeCode(ctx context.Context, code, codeVerifier string) (*oauth.TokenResponse, error)
	GetUserInfo(ctx context.Context, accessToken string) (*oauth.UserInfo, error)
}

// OAuthDeps holds dependencies for OAuth operations
type OAuthDeps struct {
	GoogleClient    OAuthProvider
	StateStore      OAuthStateStore
	OAuthIdentities OAuthIdentityRepo
}

// OAuthStartResult contains the authorization URL to redirect to
type OAuthStartResult struct {
	AuthURL string
}

// OAuthStart initiates the OAuth flow by generating state and PKCE values
func (s *Service) OAuthStart(ctx context.Context, provider, redirectTo string, deps OAuthDeps) (*OAuthStartResult, error) {
	// Validate provider
	if !domain.IsValidProvider(provider) {
		return nil, domain.New(domain.KindValidation, "unsupported_provider", "unsupported oauth provider")
	}

	// Check if provider is configured
	if provider == "google" && !deps.GoogleClient.IsConfigured() {
		return nil, domain.New(domain.KindValidation, "oauth_not_configured", "google oauth not configured")
	}

	// Generate PKCE values
	verifier, challenge, err := oauth.GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	// Store state in Redis
	stateToken, err := deps.StateStore.Create(ctx, OAuthStateData{
		CodeVerifier: verifier,
		RedirectTo:   redirectTo,
		Provider:     provider,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth state: %w", err)
	}

	// Generate authorization URL
	var authURL string
	switch provider {
	case "google":
		authURL = deps.GoogleClient.AuthURL(stateToken, challenge)
	default:
		return nil, domain.New(domain.KindValidation, "unsupported_provider", "unsupported oauth provider")
	}

	return &OAuthStartResult{AuthURL: authURL}, nil
}

// OAuthCallbackResult contains the login result after successful OAuth
type OAuthCallbackResult struct {
	User       domain.User
	Tokens     AuthTokens
	RedirectTo string
	IsNewUser  bool
}

// OAuthCallback handles the OAuth callback, exchanges code for tokens, and creates/links user
func (s *Service) OAuthCallback(ctx context.Context, provider, stateToken, code string, deps OAuthDeps) (*OAuthCallbackResult, error) {
	// Consume state (one-time use, prevents replay)
	state, err := deps.StateStore.Consume(ctx, stateToken)
	if err != nil {
		return nil, domain.New(domain.KindAuth, "invalid_oauth_state", "invalid or expired oauth state")
	}

	// Verify provider matches
	if state.Provider != provider {
		return nil, domain.New(domain.KindAuth, "provider_mismatch", "oauth provider mismatch")
	}

	// Exchange code for tokens
	var userInfo *oauth.UserInfo
	switch provider {
	case "google":
		tokenResp, err := deps.GoogleClient.ExchangeCode(ctx, code, state.CodeVerifier)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code: %w", err)
		}
		userInfo, err = deps.GoogleClient.GetUserInfo(ctx, tokenResp.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get user info: %w", err)
		}
	default:
		return nil, domain.New(domain.KindValidation, "unsupported_provider", "unsupported oauth provider")
	}

	// Look up existing OAuth identity
	identity, err := deps.OAuthIdentities.FindByProviderAndSub(ctx, provider, userInfo.Sub)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup oauth identity: %w", err)
	}

	var user domain.User
	var isNewUser bool

	if identity != nil {
		// Existing OAuth identity - login as that user
		user, err = s.users.GetByID(ctx, identity.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}

		// Check if user is locked
		if user.Locked {
			return nil, domain.ErrAccountLocked()
		}
	} else {
		// No existing OAuth identity - check email conflict
		user, isNewUser, err = s.handleOAuthEmailConflict(ctx, provider, userInfo, deps)
		if err != nil {
			return nil, err
		}
	}

	// Issue our own tokens
	tokens, err := s.issueTokens(ctx, user.ID, user.Role)
	if err != nil {
		return nil, err
	}

	// Audit log
	if isNewUser {
		s.audit("oauth_register", map[string]string{
			"user_id":  user.ID,
			"provider": provider,
			"email":    user.Email,
		})
	} else {
		s.audit("oauth_login", map[string]string{
			"user_id":  user.ID,
			"provider": provider,
		})
	}

	return &OAuthCallbackResult{
		User:       user,
		Tokens:     tokens,
		RedirectTo: state.RedirectTo,
		IsNewUser:  isNewUser,
	}, nil
}

// handleOAuthEmailConflict handles the case where OAuth user's email may conflict with existing user
func (s *Service) handleOAuthEmailConflict(ctx context.Context, provider string, userInfo *oauth.UserInfo, deps OAuthDeps) (domain.User, bool, error) {
	// Check if email already exists
	existingUser, err := s.users.GetByEmail(ctx, userInfo.Email)
	if err != nil {
		// Check if it's a "not found" error by checking the error code
		var domainErr *domain.Error
		if ok := isNotFoundError(err, &domainErr); !ok {
			return domain.User{}, false, fmt.Errorf("failed to check email: %w", err)
		}
		// Email not found - create new user
		return s.createOAuthUser(ctx, provider, userInfo, deps)
	}

	// Email exists - check if we can auto-link
	if !existingUser.EmailVerified {
		// Don't auto-link to unverified email accounts (security risk)
		return domain.User{}, false, domain.New(domain.KindValidation, "email_not_verified",
			"email already registered but not verified. please verify your email first or use password login")
	}

	// Auto-link OAuth identity to existing verified user
	identity := &domain.OAuthIdentity{
		ID:             uuid.NewString(), // Generate ID
		Provider:       provider,
		ProviderUserID: userInfo.Sub,
		Email:          userInfo.Email,
		UserID:         existingUser.ID,
	}
	if err := deps.OAuthIdentities.Create(ctx, identity); err != nil {
		return domain.User{}, false, fmt.Errorf("failed to link oauth identity: %w", err)
	}

	s.audit("oauth_linked", map[string]string{
		"user_id":  existingUser.ID,
		"provider": provider,
	})

	return existingUser, false, nil
}

// createOAuthUser creates a new user from OAuth provider info
func (s *Service) createOAuthUser(ctx context.Context, provider string, userInfo *oauth.UserInfo, deps OAuthDeps) (domain.User, bool, error) {
	// Email doesn't exist - create new user
	// Note: domain.User doesn't have Username field, so we skip it
	newUser := domain.User{
		ID:            uuid.NewString(), // Generate new ID
		Email:         userInfo.Email,
		Role:          "user",
		EmailVerified: userInfo.EmailVerified, // Trust provider's verification
		PasswordHash:  "",                     // No password for OAuth-only users
	}

	createdUser, err := s.users.Create(ctx, newUser)
	if err != nil {
		return domain.User{}, false, fmt.Errorf("failed to create user: %w", err)
	}

	// Create OAuth identity
	identity := &domain.OAuthIdentity{
		ID:             uuid.NewString(), // Generate ID
		Provider:       provider,
		ProviderUserID: userInfo.Sub,
		Email:          userInfo.Email,
		UserID:         createdUser.ID,
	}
	if err := deps.OAuthIdentities.Create(ctx, identity); err != nil {
		return domain.User{}, false, fmt.Errorf("failed to create oauth identity: %w", err)
	}

	return createdUser, true, nil
}

// isNotFoundError checks if err is a domain "not found" error
func isNotFoundError(err error, domainErr **domain.Error) bool {
	if err == nil {
		return false
	}
	// Check if it's a domain error with not_found kind
	if de, ok := err.(*domain.Error); ok {
		*domainErr = de
		return de.Kind == domain.KindNotFound
	}
	return false
}
