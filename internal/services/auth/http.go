package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/baechuer/cityevents/internal/platform/httpapi"
	"github.com/baechuer/cityevents/internal/platform/identity"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	service *Service
	cfg     config.Config
}

const (
	refreshCookieName = "cityevents_refresh"
	csrfCookieName    = "cityevents_csrf"
	csrfHeaderName    = "X-CSRF-Token"

	authJSONLimitBytes = 32 * 1024
)

func NewRouter(ctx context.Context, cfg config.Config, logger *slog.Logger) (http.Handler, func(context.Context) error, error) {
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}

	var repo Repository = NewPostgresRepository(pool)
	var closeRevocationCache func() error
	if cfg.TokenRevocationCacheEnabled {
		cache, cleanup, err := NewRedisRevocationCacheFromURL(ctx, cfg.RedisURL)
		if err != nil {
			if logger != nil {
				logger.Warn("token revocation cache unavailable; falling back to postgres", slog.String("error", err.Error()))
			}
		} else {
			repo = WithRevocationCache(repo, cache)
			closeRevocationCache = cleanup
			if logger != nil {
				logger.Info("token revocation cache enabled")
			}
		}
	}
	service := NewDefaultServiceWithRefreshTTL(repo, cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if cfg.SeedAdminEmail != "" || cfg.SeedAdminPass != "" {
		admin, err := service.EnsureSeedAdmin(ctx, SeedAdminCommand{
			Email:       cfg.SeedAdminEmail,
			Password:    cfg.SeedAdminPass,
			DisplayName: cfg.SeedAdminName,
		})
		if err != nil {
			pool.Close()
			return nil, nil, err
		}
		if logger != nil {
			logger.Info("seed admin ready", slog.String("email", admin.Email), slog.String("role", admin.Role))
		}
	}
	router := NewHTTPHandler(cfg, logger, service)

	cleanup := func(context.Context) error {
		if closeRevocationCache != nil {
			_ = closeRevocationCache()
		}
		pool.Close()
		return nil
	}
	return router, cleanup, nil
}

func NewHTTPHandler(cfg config.Config, logger *slog.Logger, service *Service) http.Handler {
	r := httpapi.NewBaseRouter(cfg, logger)
	handler := &Handler{service: service, cfg: cfg}

	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/register", handler.register)
		r.Post("/login", handler.login)
		r.Post("/refresh", handler.refresh)
		r.Get("/me", handler.me)
		r.Post("/logout", handler.logout)
		r.Patch("/users/{userID}/role", handler.updateRole)
	})

	return r
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req, authJSONLimitBytes); err != nil {
		writeDecodeError(w, err)
		return
	}

	result, err := h.service.Register(r.Context(), RegisterCommand{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.setRefreshCookie(w, result)
	health.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req, authJSONLimitBytes); err != nil {
		writeDecodeError(w, err)
		return
	}

	result, err := h.service.Login(r.Context(), LoginCommand{Email: req.Email, Password: req.Password})
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.setRefreshCookie(w, result)
	health.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, ok := refreshTokenFromCookie(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if !h.requireCSRF(w, r) {
		return
	}
	result, err := h.service.Refresh(r.Context(), refreshToken)
	if err != nil {
		h.clearRefreshCookie(w)
		writeAuthError(w, err)
		return
	}
	h.setRefreshCookie(w, result)
	health.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	user, err := h.service.CurrentUser(r.Context(), token)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string]PublicUser{"user": user})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, hasBearer := bearerToken(r)
	refreshToken, hasRefresh := refreshTokenFromCookie(r)
	if !hasBearer && !hasRefresh {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if hasRefresh && !h.requireCSRF(w, r) {
		return
	}

	var err error
	if hasBearer {
		err = h.service.Logout(r.Context(), token, refreshToken)
		if errors.Is(err, ErrUnauthorized) && hasRefresh {
			err = h.service.LogoutRefresh(r.Context(), refreshToken)
		}
	} else {
		err = h.service.LogoutRefresh(r.Context(), refreshToken)
	}
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	actor, err := h.service.CurrentUser(r.Context(), token)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	var req updateRoleRequest
	if err := decodeJSON(w, r, &req, authJSONLimitBytes); err != nil {
		writeDecodeError(w, err)
		return
	}
	updated, err := h.service.UpdateUserRole(r.Context(), UpdateRoleCommand{
		ActorUserID:  actor.ID,
		TargetUserID: chi.URLParam(r, "userID"),
		Role:         identity.NormalizeRole(req.Role),
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string]PublicUser{"user": updated})
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	return httpapi.DecodeJSONLimited(w, r, target, maxBytes)
}

func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func refreshTokenFromCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return strings.TrimSpace(cookie.Value), true
}

func csrfTokenFromCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return strings.TrimSpace(cookie.Value), true
}

func (h *Handler) requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	cookieToken, ok := csrfTokenFromCookie(r)
	headerToken := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if !ok || headerToken == "" || !hmac.Equal([]byte(cookieToken), []byte(headerToken)) {
		writeError(w, http.StatusForbidden, "csrf_failed", "CSRF token is required")
		return false
	}
	return true
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, result AuthResult) {
	if strings.TrimSpace(result.RefreshToken) == "" || result.RefreshExpiresAt.IsZero() {
		return
	}
	maxAge := int(time.Until(result.RefreshExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    result.RefreshToken,
		Path:     "/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.RefreshCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  result.RefreshExpiresAt,
		MaxAge:   maxAge,
	})
	h.setCSRFCookie(w, result.RefreshExpiresAt, maxAge)
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.RefreshCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	h.clearCSRFCookie(w)
}

func (h *Handler) setCSRFCookie(w http.ResponseWriter, expiresAt time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    newCSRFToken(),
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cfg.RefreshCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   maxAge,
	})
	h.clearCSRFCookiePath(w, "/v1/auth")
}

func (h *Handler) clearCSRFCookie(w http.ResponseWriter) {
	h.clearCSRFCookiePath(w, "/")
	h.clearCSRFCookiePath(w, "/v1/auth")
}

func (h *Handler) clearCSRFCookiePath(w http.ResponseWriter, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     path,
		HttpOnly: false,
		Secure:   h.cfg.RefreshCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func newCSRFToken() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:])
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidEmail), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrInvalidDisplayName):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrInvalidRole):
		writeError(w, http.StatusBadRequest, "invalid_role", err.Error())
	case errors.Is(err, ErrDuplicateEmail):
		writeError(w, http.StatusConflict, "duplicate_email", "email is already registered")
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "operation is not allowed")
	case errors.Is(err, ErrTooManyAttempts):
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many login attempts")
	case errors.Is(err, ErrUserNotFound):
		writeError(w, http.StatusNotFound, "not_found", "user not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, httpapi.ErrRequestBodyTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	health.WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
