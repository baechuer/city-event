package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/baechuer/cityevents/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	service *Service
}

func NewRouter(ctx context.Context, cfg config.Config, logger *slog.Logger) (http.Handler, func(context.Context) error, error) {
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}

	repo := NewPostgresRepository(pool)
	service := NewDefaultService(repo, cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL)
	router := NewHTTPHandler(cfg, logger, service)

	cleanup := func(context.Context) error {
		pool.Close()
		return nil
	}
	return router, cleanup, nil
}

func NewHTTPHandler(cfg config.Config, logger *slog.Logger, service *Service) http.Handler {
	r := httpapi.NewBaseRouter(cfg, logger)
	handler := &Handler{service: service}

	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/register", handler.register)
		r.Post("/login", handler.login)
		r.Get("/me", handler.me)
		r.Post("/logout", handler.logout)
	})

	return r
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request")
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
	health.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request")
		return
	}

	result, err := h.service.Login(r.Context(), LoginCommand{Email: req.Email, Password: req.Password})
	if err != nil {
		writeAuthError(w, err)
		return
	}
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
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	if err := h.service.Logout(r.Context(), token); err != nil {
		writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidEmail), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrInvalidDisplayName):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrDuplicateEmail):
		writeError(w, http.StatusConflict, "duplicate_email", "email is already registered")
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, ErrTooManyAttempts):
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many login attempts")
	default:
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	health.WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
