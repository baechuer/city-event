package media

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/baechuer/cityevents/internal/platform/authn"
	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/health"
	"github.com/baechuer/cityevents/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const mediaJSONLimitBytes = 64 * 1024

type Handler struct {
	service *Service
	tokens  authn.TokenManager
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
	storage, err := NewMinIOStorage(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	router := NewHTTPHandler(cfg, logger, NewService(NewPostgresRepository(pool), storage, cfg.MinIOBucket))
	cleanup := func(context.Context) error {
		pool.Close()
		return nil
	}
	return router, cleanup, nil
}

func NewHTTPHandler(cfg config.Config, logger *slog.Logger, service *Service) http.Handler {
	r := httpapi.NewBaseRouter(cfg, logger)
	handler := &Handler{
		service: service,
		tokens:  authn.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL),
	}
	r.Post("/v1/media/uploads", handler.createUpload)
	r.Post("/v1/media/{mediaID}/uploaded", handler.markUploaded)
	r.Get("/v1/media/{mediaID}", handler.getMedia)
	return r
}

func (h *Handler) createUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userIDFromRequest(r)
	if !ok {
		writeMediaError(w, ErrUnauthorized)
		return
	}
	var req createUploadRequest
	if err := decodeJSON(w, r, &req, mediaJSONLimitBytes); err != nil {
		writeDecodeError(w, err)
		return
	}
	intent, err := h.service.CreateUpload(r.Context(), UploadCommand{
		EventID:     req.EventID,
		UploaderID:  userID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
	})
	if err != nil {
		writeMediaError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusCreated, intent)
}

func (h *Handler) markUploaded(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userIDFromRequest(r)
	if !ok {
		writeMediaError(w, ErrUnauthorized)
		return
	}
	asset, err := h.service.MarkUploaded(r.Context(), chi.URLParam(r, "mediaID"), userID)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string]Asset{"asset": asset})
}

func (h *Handler) getMedia(w http.ResponseWriter, r *http.Request) {
	asset, err := h.service.Get(r.Context(), chi.URLParam(r, "mediaID"))
	if err != nil {
		writeMediaError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string]Asset{"asset": asset})
}

type createUploadRequest struct {
	EventID     string `json:"eventId"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
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

func (h *Handler) userIDFromRequest(r *http.Request) (string, bool) {
	token, ok := authn.BearerToken(r)
	if !ok {
		return "", false
	}
	claims, err := h.tokens.Verify(token)
	if err != nil {
		return "", false
	}
	return claims.UserID, claims.UserID != ""
}

func writeMediaError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "user identity required")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "operation is not allowed")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, ErrInvalidUpload), errors.Is(err, ErrInvalidContentType), errors.Is(err, ErrInvalidStateTransition):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
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
