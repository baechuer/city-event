package domain

import "time"

// OAuthIdentity links an external OAuth provider identity to a user
type OAuthIdentity struct {
	ID             string
	Provider       string // google, github, discord
	ProviderUserID string // sub/id from provider
	Email          string // cached for lookup
	UserID         string
	CreatedAt      time.Time
}

// OAuthProvider represents supported OAuth providers
type OAuthProvider string

const (
	OAuthProviderGoogle  OAuthProvider = "google"
	OAuthProviderGitHub  OAuthProvider = "github"
	OAuthProviderDiscord OAuthProvider = "discord"
)

// IsValidProvider checks if the provider is supported
func IsValidProvider(p string) bool {
	switch OAuthProvider(p) {
	case OAuthProviderGoogle, OAuthProviderGitHub, OAuthProviderDiscord:
		return true
	default:
		return false
	}
}
