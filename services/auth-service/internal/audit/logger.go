package audit

import (
	"context"

	"github.com/rs/zerolog"
)

// Logger provides structured audit logging for auth business events
type Logger struct {
	log zerolog.Logger
}

// New creates a new audit logger
func New(log zerolog.Logger) *Logger {
	return &Logger{
		log: log.With().Bool("audit", true).Logger(),
	}
}

// LoginSuccess logs a successful login
func (l *Logger) LoginSuccess(ctx context.Context, userID, email, ip string) {
	l.log.Info().
		Str("action", "login_success").
		Str("user_id", userID).
		Str("email", maskEmail(email)).
		Str("ip", ip).
		Str("request_id", getRequestID(ctx)).
		Msg("User logged in successfully")
}

// LoginFailed logs a failed login attempt
func (l *Logger) LoginFailed(ctx context.Context, email, ip, reason string) {
	l.log.Warn().
		Str("action", "login_failed").
		Str("email", maskEmail(email)).
		Str("ip", ip).
		Str("reason", reason).
		Str("request_id", getRequestID(ctx)).
		Msg("Login attempt failed")
}

// TokenRefreshed logs a token refresh
func (l *Logger) TokenRefreshed(ctx context.Context, userID string) {
	l.log.Info().
		Str("action", "token_refreshed").
		Str("user_id", userID).
		Str("request_id", getRequestID(ctx)).
		Msg("Access token refreshed")
}

// Logout logs a user logout
func (l *Logger) Logout(ctx context.Context, userID string) {
	l.log.Info().
		Str("action", "logout").
		Str("user_id", userID).
		Str("request_id", getRequestID(ctx)).
		Msg("User logged out")
}

// SessionsRevoked logs when all sessions are revoked
func (l *Logger) SessionsRevoked(ctx context.Context, userID string) {
	l.log.Warn().
		Str("action", "sessions_revoked").
		Str("user_id", userID).
		Str("request_id", getRequestID(ctx)).
		Msg("All sessions revoked for user")
}

// PasswordChanged logs a password change
func (l *Logger) PasswordChanged(ctx context.Context, userID string) {
	l.log.Info().
		Str("action", "password_changed").
		Str("user_id", userID).
		Str("request_id", getRequestID(ctx)).
		Msg("User password changed")
}

// PasswordResetRequested logs a password reset request
func (l *Logger) PasswordResetRequested(ctx context.Context, email string) {
	l.log.Info().
		Str("action", "password_reset_requested").
		Str("email", maskEmail(email)).
		Str("request_id", getRequestID(ctx)).
		Msg("Password reset requested")
}

// EmailVerified logs when email is verified
func (l *Logger) EmailVerified(ctx context.Context, userID, email string) {
	l.log.Info().
		Str("action", "email_verified").
		Str("user_id", userID).
		Str("email", maskEmail(email)).
		Str("request_id", getRequestID(ctx)).
		Msg("Email verified")
}

// UserBanned logs when a user is banned
func (l *Logger) UserBanned(ctx context.Context, targetID, actorID, reason string) {
	l.log.Warn().
		Str("action", "user_banned").
		Str("target_user_id", targetID).
		Str("actor_user_id", actorID).
		Str("reason", reason).
		Str("request_id", getRequestID(ctx)).
		Msg("User banned")
}

// UserUnbanned logs when a user is unbanned
func (l *Logger) UserUnbanned(ctx context.Context, targetID, actorID string) {
	l.log.Info().
		Str("action", "user_unbanned").
		Str("target_user_id", targetID).
		Str("actor_user_id", actorID).
		Str("request_id", getRequestID(ctx)).
		Msg("User unbanned")
}

// RoleChanged logs when a user's role is changed
func (l *Logger) RoleChanged(ctx context.Context, targetID, actorID, oldRole, newRole string) {
	l.log.Warn().
		Str("action", "role_changed").
		Str("target_user_id", targetID).
		Str("actor_user_id", actorID).
		Str("old_role", oldRole).
		Str("new_role", newRole).
		Str("request_id", getRequestID(ctx)).
		Msg("User role changed")
}

// maskEmail partially masks email for privacy in logs
func maskEmail(email string) string {
	if len(email) < 5 {
		return "***"
	}
	// Show first 2 chars and domain
	at := 0
	for i, c := range email {
		if c == '@' {
			at = i
			break
		}
	}
	if at < 2 {
		return email[:1] + "***" + email[at:]
	}
	return email[:2] + "***" + email[at:]
}

// getRequestID extracts request ID from context
func getRequestID(ctx context.Context) string {
	if v := ctx.Value("request_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
