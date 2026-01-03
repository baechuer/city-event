package dto

// -------- Core auth --------

type RegisterResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"` // "Bearer"
	ExpiresIn   int64  `json:"expires_in"` // seconds
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// Logout typically returns 204 No Content, so response body is optional/unused.
type LogoutResponse struct {
	Status string `json:"status,omitempty"` // "ok"
}

// -------- Me / Admin --------

type MeResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

type AdminResponse struct {
	Message string `json:"message"`
}

// -------- Email verification --------

type VerifyEmailConfirmResponse struct {
	Status string `json:"status"` // "verified"
}

// -------- Password reset --------

type PasswordResetRequestResponse struct {
	Status string `json:"status"` // "ok"
}

type PasswordResetConfirmResponse struct {
	Status string `json:"status"` // "ok"
}

type PasswordResetValidateResponse struct {
	Valid bool `json:"valid"`
}

// -------- Password change --------

type PasswordChangeResponse struct {
	Status string `json:"status"` // "ok"
}

// -------- Sessions --------

type SessionsRevokeResponse struct {
	Status string `json:"status"` // "ok"
}

// -------- Optional --------

type MeStatusResponse struct {
	EmailVerified bool   `json:"email_verified"`
	Locked        bool   `json:"locked"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	Role          string `json:"role,omitempty"`
}

// UserView is the standard user payload for auth-service responses.
type UserView struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	EmailVerified bool   `json:"email_verified"`
	Locked        bool   `json:"locked"`
	HasPassword   bool   `json:"has_password"`
}

// TokensView is the standard access token payload.
// (Refresh token is stored in HttpOnly cookie, so we never return it in JSON.)
type TokensView struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"` // "Bearer"
	ExpiresIn   int64  `json:"expires_in"` // seconds
}

// AuthData is returned by register/login.
type AuthData struct {
	User   UserView   `json:"user"`
	Tokens TokensView `json:"tokens"`
}

// RefreshData is returned by refresh.
type RefreshData struct {
	Tokens TokensView `json:"tokens"`
	User   UserView   `json:"user"`
}

// MeData is returned by /me.
type MeData struct {
	User UserView `json:"user"`
}

// AdminData is returned by /admin.
type AdminData struct {
	Message string   `json:"message"`
	User    UserView `json:"user"`
}
