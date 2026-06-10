package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/baechuer/cityevents/internal/platform/httpapi"
	"github.com/baechuer/cityevents/internal/platform/identity"
)

var (
	errGatewayUnauthorized = errors.New("authentication required")
	errGatewayUpstream     = errors.New("upstream request failed")
)

type Handler struct {
	authURL    *url.URL
	authProxy  *httputil.ReverseProxy
	eventProxy *httputil.ReverseProxy
	feedProxy  *httputil.ReverseProxy
	mediaProxy *httputil.ReverseProxy
	client     *http.Client
	logger     *slog.Logger
}

type principal struct {
	UserID string
	Role   identity.Role
}

func NewRouter(_ context.Context, cfg config.Config, logger *slog.Logger) (http.Handler, func(context.Context) error, error) {
	router, err := NewHTTPHandler(cfg, logger, http.DefaultTransport)
	if err != nil {
		return nil, nil, err
	}
	return router, nil, nil
}

func NewHTTPHandler(cfg config.Config, logger *slog.Logger, transport http.RoundTripper) (http.Handler, error) {
	authURL, err := url.Parse(cfg.AuthServiceURL)
	if err != nil {
		return nil, err
	}
	eventURL, err := url.Parse(cfg.EventServiceURL)
	if err != nil {
		return nil, err
	}
	feedURL, err := url.Parse(cfg.FeedServiceURL)
	if err != nil {
		return nil, err
	}
	mediaURL, err := url.Parse(cfg.MediaServiceURL)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	if logger == nil {
		logger = slog.Default()
	}

	handler := &Handler{
		authURL:    authURL,
		authProxy:  newProxy(authURL, logger),
		eventProxy: newProxy(eventURL, logger),
		feedProxy:  newProxy(feedURL, logger),
		mediaProxy: newProxy(mediaURL, logger),
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		},
		logger: logger,
	}

	r := httpapi.NewBaseRouter(cfg, logger)
	r.Handle("/v1/auth/*", http.HandlerFunc(handler.proxyAuth))
	r.Handle("/v1/events", http.HandlerFunc(handler.proxyEvents))
	r.Handle("/v1/events/*", http.HandlerFunc(handler.proxyEvents))
	r.Handle("/v1/feed/*", http.HandlerFunc(handler.proxyPublic(handler.feedProxy)))
	r.Handle("/v1/media/*", http.HandlerFunc(handler.proxyMedia))
	return r, nil
}

func newProxy(target *url.URL, logger *slog.Logger) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if logger != nil {
			logger.Error("gateway proxy failed", slog.String("target", target.Host), slog.String("error", err.Error()))
		}
		writeGatewayError(w, http.StatusBadGateway, "bad_gateway", "upstream service unavailable")
	}
	return proxy
}

func (h *Handler) proxyAuth(w http.ResponseWriter, r *http.Request) {
	h.authProxy.ServeHTTP(w, sanitizedRequest(r))
}

func (h *Handler) proxyPublic(proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, sanitizedRequest(r))
	}
}

func (h *Handler) proxyEvents(w http.ResponseWriter, r *http.Request) {
	switch eventAuthMode(r) {
	case authNotRequired:
		h.eventProxy.ServeHTTP(w, sanitizedRequest(r))
	case authOptional:
		token, ok := bearerToken(r)
		if !ok {
			h.eventProxy.ServeHTTP(w, sanitizedRequest(r))
			return
		}
		user, err := h.authenticate(r.Context(), token)
		if err != nil {
			writeGatewayAuthError(w, err)
			return
		}
		h.eventProxy.ServeHTTP(w, requestWithPrincipal(r, user))
	default:
		user, err := h.requirePrincipal(r)
		if err != nil {
			writeGatewayAuthError(w, err)
			return
		}
		h.eventProxy.ServeHTTP(w, requestWithPrincipal(r, user))
	}
}

func (h *Handler) proxyMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.mediaProxy.ServeHTTP(w, sanitizedRequest(r))
		return
	}
	user, err := h.requirePrincipal(r)
	if err != nil {
		writeGatewayAuthError(w, err)
		return
	}
	h.mediaProxy.ServeHTTP(w, requestWithPrincipal(r, user))
}

func (h *Handler) requirePrincipal(r *http.Request) (principal, error) {
	token, ok := bearerToken(r)
	if !ok {
		return principal{}, errGatewayUnauthorized
	}
	return h.authenticate(r.Context(), token)
}

func (h *Handler) authenticate(ctx context.Context, token string) (principal, error) {
	meURL := h.authURL.ResolveReference(&url.URL{Path: "/v1/auth/me"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meURL.String(), nil)
	if err != nil {
		return principal{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.client.Do(req)
	if err != nil {
		return principal{}, errGatewayUpstream
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return principal{}, errGatewayUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return principal{}, errGatewayUpstream
	}

	var payload struct {
		User struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return principal{}, errGatewayUpstream
	}
	role := identity.NormalizeRole(payload.User.Role)
	if strings.TrimSpace(payload.User.ID) == "" || !identity.ValidRole(role) {
		return principal{}, errGatewayUnauthorized
	}
	return principal{UserID: payload.User.ID, Role: role}, nil
}

type authMode int

const (
	authNotRequired authMode = iota
	authOptional
	authRequired
)

func eventAuthMode(r *http.Request) authMode {
	if r.Method != http.MethodGet {
		return authRequired
	}
	remaining := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/events"), "/")
	if remaining == "" {
		return authNotRequired
	}
	if !strings.Contains(remaining, "/") {
		return authOptional
	}
	return authRequired
}

func requestWithPrincipal(r *http.Request, user principal) *http.Request {
	req := sanitizedRequest(r)
	req.Header.Set(identity.HeaderUserID, user.UserID)
	req.Header.Set(identity.HeaderUserRole, string(user.Role))
	return req
}

func sanitizedRequest(r *http.Request) *http.Request {
	req := r.Clone(r.Context())
	req.Body = r.Body
	req.Header = r.Header.Clone()
	req.Header.Del(identity.HeaderUserID)
	req.Header.Del(identity.HeaderUserRole)
	return req
}

func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func writeGatewayAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errGatewayUnauthorized):
		writeGatewayError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, errGatewayUpstream):
		writeGatewayError(w, http.StatusBadGateway, "bad_gateway", "authentication service unavailable")
	default:
		writeGatewayError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeGatewayError(w http.ResponseWriter, status int, code, message string) {
	health.WriteJSON(w, status, map[string]map[string]string{
		"error": {
			"code":    code,
			"message": message,
		},
	})
}
