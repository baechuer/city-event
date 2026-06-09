package feed

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
	redisClient, err := NewRedisClient(cfg.RedisURL)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}

	service := NewService(NewPostgresRepository(pool), NewRedisCache(redisClient))
	router := NewHTTPHandler(cfg, logger, service)
	cleanup := func(context.Context) error {
		_ = redisClient.Close()
		pool.Close()
		return nil
	}
	return router, cleanup, nil
}

func NewHTTPHandler(cfg config.Config, logger *slog.Logger, service *Service) http.Handler {
	r := httpapi.NewBaseRouter(cfg, logger)
	handler := &Handler{service: service}

	r.Get("/v1/feed/events", handler.listEvents)
	r.Get("/v1/feed/events/{eventID}", handler.getEvent)
	return r
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	query, err := parseFeedQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	events, err := h.service.ListEvents(r.Context(), query)
	if err != nil {
		writeFeedError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string][]Event{"events": events})
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	event, err := h.service.GetEvent(r.Context(), chi.URLParam(r, "eventID"))
	if err != nil {
		writeFeedError(w, err)
		return
	}
	health.WriteJSON(w, http.StatusOK, map[string]Event{"event": event})
}

func parseFeedQuery(r *http.Request) (Query, error) {
	values := r.URL.Query()
	limit, err := parseOptionalInt(values.Get("limit"), DefaultLimit)
	if err != nil {
		return Query{}, errors.New("limit must be an integer")
	}
	offset, err := parseOptionalInt(values.Get("offset"), 0)
	if err != nil {
		return Query{}, errors.New("offset must be an integer")
	}
	if limit < 0 {
		return Query{}, errors.New("limit must be non-negative")
	}
	if offset < 0 {
		return Query{}, errors.New("offset must be non-negative")
	}
	return NormalizeQuery(Query{
		City:   strings.TrimSpace(values.Get("city")),
		Limit:  limit,
		Offset: offset,
	}), nil
}

func parseOptionalInt(value string, fallback int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeFeedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	health.WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
