package http_handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/security"
	"github.com/go-chi/chi/v5"
)

// OAuthHandler handles OAuth endpoints
type OAuthHandler struct {
	svc              *auth.Service
	googleClient     auth.OAuthProvider
	stateStore       auth.OAuthStateStore
	oauthIdentities  auth.OAuthIdentityRepo
	frontendOrigin   string
	allowedRedirects []string
	isSecure         bool // Whether to use secure cookies
}

// OAuthHandlerConfig holds configuration for OAuth handler
type OAuthHandlerConfig struct {
	Service          *auth.Service
	GoogleClient     auth.OAuthProvider
	StateStore       auth.OAuthStateStore
	OAuthIdentities  auth.OAuthIdentityRepo
	FrontendOrigin   string
	AllowedRedirects []string
	IsSecure         bool
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(cfg OAuthHandlerConfig) *OAuthHandler {
	return &OAuthHandler{
		svc:              cfg.Service,
		googleClient:     cfg.GoogleClient,
		stateStore:       cfg.StateStore,
		oauthIdentities:  cfg.OAuthIdentities,
		frontendOrigin:   cfg.FrontendOrigin,
		allowedRedirects: cfg.AllowedRedirects,
		isSecure:         cfg.IsSecure,
	}
}

// OAuthStart initiates OAuth flow by redirecting to provider
// GET /auth/v1/oauth/{provider}/start?redirect_to=/events
func (h *OAuthHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	redirectTo := r.URL.Query().Get("redirect_to")

	// Validate redirect_to against whitelist
	if !h.isAllowedRedirect(redirectTo) {
		redirectTo = "/"
	}

	deps := auth.OAuthDeps{
		GoogleClient:    h.googleClient,
		StateStore:      h.stateStore,
		OAuthIdentities: h.oauthIdentities,
	}

	result, err := h.svc.OAuthStart(r.Context(), provider, redirectTo, deps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Redirect to provider's authorization page
	http.Redirect(w, r, result.AuthURL, http.StatusFound)
}

// OAuthCallback handles the OAuth callback from provider
// GET /auth/v1/oauth/{provider}/callback?code=...&state=...
func (h *OAuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	code := r.URL.Query().Get("code")
	stateToken := r.URL.Query().Get("state")

	// Check for OAuth errors
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		errDesc := r.URL.Query().Get("error_description")
		h.renderErrorPage(w, fmt.Sprintf("OAuth error: %s - %s", errCode, errDesc))
		return
	}

	if code == "" || stateToken == "" {
		h.renderErrorPage(w, "Missing code or state parameter")
		return
	}

	deps := auth.OAuthDeps{
		GoogleClient:    h.googleClient,
		StateStore:      h.stateStore,
		OAuthIdentities: h.oauthIdentities,
	}

	result, err := h.svc.OAuthCallback(r.Context(), provider, stateToken, code, deps)
	if err != nil {
		h.renderErrorPage(w, err.Error())
		return
	}

	// Set HttpOnly refresh token cookie (7 days default TTL)
	security.SetRefreshToken(w, result.Tokens.RefreshToken, 7*24*time.Hour, h.isSecure)

	// Render postMessage page to pass access token to opener
	h.renderPostMessagePage(w, result)
}

// isAllowedRedirect checks if the redirect path is in the whitelist
func (h *OAuthHandler) isAllowedRedirect(path string) bool {
	if path == "" {
		return false
	}
	return slices.Contains(h.allowedRedirects, path)
}

// postMessageData holds data for the postMessage template
type postMessageData struct {
	Origin      string
	AccessToken string
	ExpiresIn   int64
	UserJSON    template.JS // Safe JSON string
	RedirectTo  string
}

// renderPostMessagePage renders HTML that posts the token to the opener window
func (h *OAuthHandler) renderPostMessagePage(w http.ResponseWriter, result *auth.OAuthCallbackResult) {
	// Prepare user data for JSON
	userData := map[string]interface{}{
		"id":             result.User.ID,
		"email":          result.User.Email,
		"role":           result.User.Role,
		"email_verified": result.User.EmailVerified,
	}
	userJSON, _ := json.Marshal(userData)

	data := postMessageData{
		Origin:      h.frontendOrigin,
		AccessToken: result.Tokens.AccessToken,
		ExpiresIn:   result.Tokens.ExpiresIn,
		UserJSON:    template.JS(userJSON),
		RedirectTo:  result.RedirectTo,
	}

	tmpl := template.Must(template.New("postmessage").Parse(postMessageTemplate))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

// renderErrorPage renders an error page
func (h *OAuthHandler) renderErrorPage(w http.ResponseWriter, message string) {
	data := struct {
		Origin  string
		Message string
	}{
		Origin:  h.frontendOrigin,
		Message: message,
	}

	tmpl := template.Must(template.New("error").Parse(errorTemplate))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	w.Write(buf.Bytes())
}

const postMessageTemplate = `<!DOCTYPE html>
<html>
<head>
  <title>Login Complete</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
    .container { text-align: center; padding: 2rem; background: white; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
    .spinner { border: 3px solid #f3f3f3; border-top: 3px solid #3498db; border-radius: 50%; width: 30px; height: 30px; animation: spin 1s linear infinite; margin: 0 auto 1rem; }
    @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="container">
    <div class="spinner"></div>
    <p>Completing login...</p>
  </div>
  <script>
    (function() {
      const origin = '{{.Origin}}';
      const data = {
        type: 'oauth_success',
        access_token: '{{.AccessToken}}',
        expires_in: {{.ExpiresIn}},
        user: {{.UserJSON}},
        redirect_to: '{{.RedirectTo}}'
      };
      
      if (window.opener) {
        window.opener.postMessage(data, origin);
        window.close();
      } else {
        // Fallback: redirect to frontend with data in sessionStorage
        sessionStorage.setItem('oauth_result', JSON.stringify(data));
        window.location.href = origin + (data.redirect_to || '/');
      }
    })();
  </script>
</body>
</html>`

const errorTemplate = `<!DOCTYPE html>
<html>
<head>
  <title>Login Failed</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
    .container { text-align: center; padding: 2rem; background: white; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 400px; }
    .error { color: #e74c3c; }
    button { margin-top: 1rem; padding: 0.5rem 1rem; cursor: pointer; }
  </style>
</head>
<body>
  <div class="container">
    <h2 class="error">Login Failed</h2>
    <p>{{.Message}}</p>
    <button onclick="window.close()">Close</button>
  </div>
  <script>
    if (window.opener) {
      window.opener.postMessage({ type: 'oauth_error', error: '{{.Message}}' }, '{{.Origin}}');
    }
  </script>
</body>
</html>`
