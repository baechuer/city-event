package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/bff-service/internal/downstream"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockEventClient struct {
	mock.Mock
}

func (m *mockEventClient) GetEvent(ctx context.Context, eventID uuid.UUID) (*domain.Event, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) GetOwnEvent(ctx context.Context, eventID uuid.UUID, bearerToken string) (*domain.Event, error) {
	args := m.Called(ctx, eventID, bearerToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) ListEvents(ctx context.Context, query url.Values) (*domain.PaginatedResponse[domain.EventCard], error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaginatedResponse[domain.EventCard]), args.Error(1)
}

func (m *mockEventClient) CreateEvent(ctx context.Context, bearerToken string, body interface{}) (*domain.Event, error) {
	args := m.Called(ctx, bearerToken, body)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) PublishEvent(ctx context.Context, bearerToken, eventID string) (*domain.Event, error) {
	args := m.Called(ctx, bearerToken, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) UpdateEvent(ctx context.Context, bearerToken, eventID string, body interface{}) (*domain.Event, error) {
	args := m.Called(ctx, bearerToken, eventID, body)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) CancelEvent(ctx context.Context, bearerToken, eventID string, body interface{}) (*domain.Event, error) {
	args := m.Called(ctx, bearerToken, eventID, body)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) UnpublishEvent(ctx context.Context, bearerToken, eventID string, body interface{}) (*domain.Event, error) {
	args := m.Called(ctx, bearerToken, eventID, body)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *mockEventClient) ListMine(ctx context.Context, bearerToken string, query url.Values) (*domain.PaginatedResponse[domain.EventCard], error) {
	args := m.Called(ctx, bearerToken, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaginatedResponse[domain.EventCard]), args.Error(1)
}

func (m *mockEventClient) GetCitySuggestions(ctx context.Context, query string) ([]string, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

type mockJoinClient struct {
	mock.Mock
}

func (m *mockJoinClient) GetParticipation(ctx context.Context, eventID, userID uuid.UUID, bearerToken string) (*domain.Participation, error) {
	args := m.Called(ctx, eventID, userID, bearerToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Participation), args.Error(1)
}

func (m *mockJoinClient) JoinEvent(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) (domain.ParticipationStatus, error) {
	args := m.Called(ctx, eventID, bearerToken, idempotencyKey, requestID)
	return args.Get(0).(domain.ParticipationStatus), args.Error(1)
}

func (m *mockJoinClient) CancelJoin(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) error {
	args := m.Called(ctx, eventID, bearerToken, idempotencyKey, requestID)
	return args.Error(0)
}

func (m *mockJoinClient) ListMyJoins(ctx context.Context, bearerToken string, query url.Values) (*domain.PaginatedResponse[domain.JoinRecord], error) {
	args := m.Called(ctx, bearerToken, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaginatedResponse[domain.JoinRecord]), args.Error(1)
}

type mockAuthClient struct {
	mock.Mock
}

func (m *mockAuthClient) GetUser(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func TestGetEventView_Success(t *testing.T) {
	ec := new(mockEventClient)
	jc := new(mockJoinClient)
	ac := new(mockAuthClient)
	h := NewEventHandler(ec, jc, ac)

	eventID := uuid.New()
	userID := uuid.New()

	event := &domain.Event{ID: eventID, Title: "Test Event", StartTime: time.Now().Add(24 * time.Hour)}
	part := &domain.Participation{EventID: eventID, UserID: userID, Status: domain.StatusActive}

	ec.On("GetEvent", mock.Anything, eventID).Return(event, nil)
	jc.On("GetParticipation", mock.Anything, eventID, userID, mock.Anything).Return(part, nil)
	ac.On("GetUser", mock.Anything, mock.Anything).Return(&domain.User{Email: "test@example.com"}, nil)

	req := httptest.NewRequest("GET", "/api/events/"+eventID.String()+"/view", nil)

	// Add chi context
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", eventID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Inject UserID into context
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.GetEventView(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res EventViewResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	assert.NoError(t, err)
	assert.Equal(t, "Test Event", res.Event.Title)
	assert.Equal(t, domain.StatusActive, res.Participation.Status)
	assert.Nil(t, res.Degraded)
	assert.True(t, res.Actions.CanCancel)
}

func TestGetEventView_Degraded(t *testing.T) {
	ec := new(mockEventClient)
	jc := new(mockJoinClient)
	ac := new(mockAuthClient)
	h := NewEventHandler(ec, jc, ac)

	eventID := uuid.New()
	userID := uuid.New()
	event := &domain.Event{ID: eventID, Title: "Test Event", StartTime: time.Now().Add(24 * time.Hour)}

	ec.On("GetEvent", mock.Anything, eventID).Return(event, nil)
	jc.On("GetParticipation", mock.Anything, eventID, userID, mock.Anything).Return(nil, downstream.ErrTimeout)
	ac.On("GetUser", mock.Anything, mock.Anything).Return(&domain.User{Email: "test@example.com"}, nil)

	req := httptest.NewRequest("GET", "/api/events/"+eventID.String()+"/view", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", eventID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Inject UserID into context
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.GetEventView(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res EventViewResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	assert.NoError(t, err)
	assert.Equal(t, "Test Event", res.Event.Title)
	assert.Nil(t, res.Participation)
	assert.NotNil(t, res.Degraded)
	assert.Equal(t, "timeout", res.Degraded.Participation)
	assert.False(t, res.Actions.CanJoin)
	assert.Equal(t, "participation_unavailable", res.Actions.Reason)
}

func TestGetEventView_EventNotFound(t *testing.T) {
	ec := new(mockEventClient)
	jc := new(mockJoinClient)
	ac := new(mockAuthClient)
	h := NewEventHandler(ec, jc, ac)

	eventID := uuid.New()
	ec.On("GetEvent", mock.Anything, eventID).Return(nil, downstream.ErrNotFound)
	jc.On("GetParticipation", mock.Anything, eventID, mock.Anything, mock.Anything).Return(&domain.Participation{Status: domain.StatusNone}, nil)

	req := httptest.NewRequest("GET", "/api/events/"+eventID.String()+"/view", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", eventID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetEventView(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var apiErr domain.APIError
	json.Unmarshal(w.Body.Bytes(), &apiErr)
	assert.Equal(t, "resource_not_found", apiErr.Error.Code)
}

func TestListCreatedEvents_Success(t *testing.T) {
	ec := new(mockEventClient)
	h := NewEventHandler(ec, nil, nil)

	userID := uuid.New()
	token := "valid-token"

	cards := []domain.EventCard{
		{
			ID:                 uuid.New(),
			Title:              "My Event",
			ActiveParticipants: 5,
		},
	}
	resp := &domain.PaginatedResponse[domain.EventCard]{
		Items:      cards,
		NextCursor: "anything", // will be overridden by handler
		HasMore:    true,
	}

	ec.On("ListMine", mock.Anything, "Bearer "+token, mock.Anything).Return(resp, nil)

	req := httptest.NewRequest("GET", "/api/me/events?page_size=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	// Inject UserID and BearerToken into context
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.BearerTokenKey, "Bearer "+token)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.ListCreatedEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result domain.PaginatedResponse[domain.EventCard]
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "My Event", result.Items[0].Title)
	assert.Equal(t, 5, result.Items[0].ActiveParticipants)
	assert.Equal(t, "2", result.NextCursor)
}
