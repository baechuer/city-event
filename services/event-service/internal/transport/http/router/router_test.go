package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/config"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/handlers"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/middleware"
	"github.com/stretchr/testify/assert"
)

// stubClock prevents nil pointer panic in handlers
type stubClock struct{}

func (stubClock) Now() time.Time { return time.Date(2025, 12, 26, 12, 0, 0, 0, time.UTC) }

// stubRepo must implement all methods of event.EventRepo
type stubRepo struct{}

func (s *stubRepo) Create(ctx context.Context, e *domain.Event) error { return nil }
func (s *stubRepo) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	return &domain.Event{Status: domain.StatusPublished}, nil
}
func (s *stubRepo) Update(ctx context.Context, e *domain.Event) error { return nil }
func (s *stubRepo) ListByOwner(ctx context.Context, o, status string, p, ps int) ([]*domain.Event, int, error) {
	return []*domain.Event{}, 0, nil
}

func (s *stubRepo) ListPublicTimeKeyset(ctx context.Context, f event.ListFilter, hasCursor bool, afterStart time.Time, afterID string) ([]*domain.Event, error) {
	return []*domain.Event{}, nil
}

func (s *stubRepo) ListPublicRelevanceKeyset(ctx context.Context, f event.ListFilter, hasCursor bool, afterRank float64, afterStart time.Time, afterID string) ([]*domain.Event, []float64, error) {
	return []*domain.Event{}, []float64{}, nil
}

func (s *stubRepo) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return nil
}
func (s *stubRepo) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return nil
}
func (s *stubRepo) GetCitySuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	return []string{}, nil
}
func (s *stubRepo) GetByIDs(ctx context.Context, ids []string) ([]*domain.Event, error) {
	return []*domain.Event{}, nil
}

// FIX: Added WithTx to satisfy the EventRepo interface
func (s *stubRepo) WithTx(ctx context.Context, fn func(r event.TxEventRepo) error) error {
	return fn(&stubTxRepo{})
}

// stubTxRepo handles transaction-specific repository calls
type stubTxRepo struct{}

func (s *stubTxRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Event, error) {
	return &domain.Event{}, nil
}
func (s *stubTxRepo) Update(ctx context.Context, e *domain.Event) error               { return nil }
func (s *stubTxRepo) InsertOutbox(ctx context.Context, msg event.OutboxMessage) error { return nil }

func TestRouter_Routing(t *testing.T) {
	authMw := middleware.NewAuth("secret", "issuer", nil)
	clock := stubClock{}

	// FIX: Corrected event.New signature:
	// want: (repo, clock, cache, ttlDetails, ttlList)
	svc := event.New(&stubRepo{}, clock, nil, 0, 0)

	h := handlers.NewEventsHandler(svc, clock)
	z := handlers.NewHealthHandler()

	cfg := &config.Config{
		RLEnabled: false,
	}

	// New(h, auth, z, rdb, cfg)
	r := New(h, authMw, z, nil, cfg)

	t.Run("public_route_returns_200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/event/v1/events", nil)
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("protected_route_returns_401_without_token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/event/v1/events", nil)
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}
