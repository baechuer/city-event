package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// MockClock for stable testing
type mockClock struct{ t time.Time }

func (m mockClock) Now() time.Time { return m.t }

// TestEventsHandler_GetPublic
func TestEventsHandler_GetPublic(t *testing.T) {
	// 1. Setup
	now := time.Now().UTC()
	repo := &mockRepo{}

	// FIX: event.New requires (Repo, Clock, Cache, ttlDetails, ttlList)
	// We pass nil for cache and 0 for durations to use defaults
	svc := event.New(repo, mockClock{t: now}, nil, 0, 0)
	h := NewEventsHandler(svc, mockClock{t: now})

	t.Run("return_400_on_invalid_uuid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events/invalid-uuid", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("event_id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		h.GetPublic(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "validation_error")
	})
}

// Full mock repo satisfying the event.EventRepo interface
type mockRepo struct{}

func (m *mockRepo) Create(ctx context.Context, e *domain.Event) error { return nil }
func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	if id == "not-found" {
		return nil, domain.ErrNotFound("not found")
	}
	return &domain.Event{
		ID:        id,
		Status:    domain.StatusPublished,
		StartTime: time.Now().Add(time.Hour),
		EndTime:   time.Now().Add(2 * time.Hour),
	}, nil
}
func (m *mockRepo) Update(ctx context.Context, e *domain.Event) error { return nil }
func (m *mockRepo) ListByOwner(ctx context.Context, ownerID string, status string, page, pageSize int) ([]*domain.Event, int, error) {
	return nil, 0, nil
}

// Satisfy Keyset pagination requirements
func (m *mockRepo) ListPublicTimeKeyset(ctx context.Context, f event.ListFilter, hasCursor bool, afterStart time.Time, afterID string) ([]*domain.Event, error) {
	return []*domain.Event{}, nil
}

func (m *mockRepo) ListPublicRelevanceKeyset(ctx context.Context, f event.ListFilter, hasCursor bool, afterRank float64, afterStart time.Time, afterID string) ([]*domain.Event, []float64, error) {
	return []*domain.Event{}, []float64{}, nil
}

func (m *mockRepo) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return nil
}
func (m *mockRepo) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return nil
}
func (m *mockRepo) GetCitySuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	return []string{}, nil
}

// Satisfy Transaction requirements
func (m *mockRepo) WithTx(ctx context.Context, fn func(r event.TxEventRepo) error) error {
	return fn(&mockTxRepo{})
}

type mockTxRepo struct{}

func (m *mockTxRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Event, error) {
	return &domain.Event{ID: id}, nil
}
func (m *mockTxRepo) Update(ctx context.Context, e *domain.Event) error               { return nil }
func (m *mockTxRepo) InsertOutbox(ctx context.Context, msg event.OutboxMessage) error { return nil }
