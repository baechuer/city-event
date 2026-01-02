package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock" // Keep mock for MockCache, but MockRepo will change
)

type MockRepo struct{ mock.Mock }

func (m *MockRepo) JoinEvent(ctx context.Context, tid, idempotencyKey string, eid, uid uuid.UUID) (domain.JoinStatus, error) {
	args := m.Called(ctx, tid, idempotencyKey, eid, uid)
	return args.Get(0).(domain.JoinStatus), args.Error(1)
}
func (m *MockRepo) CancelJoin(ctx context.Context, tid, idempotencyKey string, eid, uid uuid.UUID) error {
	return m.Called(ctx, tid, idempotencyKey, eid, uid).Error(0)
}
func (m *MockRepo) GetByEventAndUser(ctx context.Context, eventID, userID uuid.UUID) (domain.JoinRecord, error) {
	args := m.Called(ctx, eventID, userID)
	return args.Get(0).(domain.JoinRecord), args.Error(1)
}
func (m *MockRepo) GetEventOwnerID(ctx context.Context, eid uuid.UUID) (uuid.UUID, error) {
	args := m.Called(ctx, eid)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

// Reads
func (m *MockRepo) ListMyJoins(ctx context.Context, u uuid.UUID, s []domain.JoinStatus, f, t *time.Time, l int, c *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	args := m.Called(ctx, u, s, f, t, l, c)
	var recs []domain.JoinRecord
	if v := args.Get(0); v != nil {
		recs = v.([]domain.JoinRecord)
	}
	var next *domain.KeysetCursor
	if v := args.Get(1); v != nil {
		next = v.(*domain.KeysetCursor)
	}
	return recs, next, args.Error(2)
}
func (m *MockRepo) ListParticipants(ctx context.Context, e uuid.UUID, l int, c *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	args := m.Called(ctx, e, l, c)
	var recs []domain.JoinRecord
	if v := args.Get(0); v != nil {
		recs = v.([]domain.JoinRecord)
	}
	var next *domain.KeysetCursor
	if v := args.Get(1); v != nil {
		next = v.(*domain.KeysetCursor)
	}
	return recs, next, args.Error(2)
}
func (m *MockRepo) ListWaitlist(ctx context.Context, e uuid.UUID, l int, c *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	args := m.Called(ctx, e, l, c)
	var recs []domain.JoinRecord
	if v := args.Get(0); v != nil {
		recs = v.([]domain.JoinRecord)
	}
	var next *domain.KeysetCursor
	if v := args.Get(1); v != nil {
		next = v.(*domain.KeysetCursor)
	}
	return recs, next, args.Error(2)
}
func (m *MockRepo) GetStats(ctx context.Context, e uuid.UUID) (domain.EventStats, error) {
	args := m.Called(ctx, e)
	return args.Get(0).(domain.EventStats), args.Error(1)
}

// Moderation
func (m *MockRepo) Kick(ctx context.Context, tid string, eid, target, actor uuid.UUID, reason string) error {
	return m.Called(ctx, tid, eid, target, actor, reason).Error(0)
}
func (m *MockRepo) Ban(ctx context.Context, tid string, eid, target, actor uuid.UUID, reason string, exp *time.Time) error {
	return m.Called(ctx, tid, eid, target, actor, reason, exp).Error(0)
}
func (m *MockRepo) Unban(ctx context.Context, tid string, eid, target, actor uuid.UUID) error {
	return m.Called(ctx, tid, eid, target, actor).Error(0)
}

// Existing (consumer paths)
func (m *MockRepo) InitCapacity(ctx context.Context, eid uuid.UUID, cap int) error {
	return m.Called(ctx, eid, cap).Error(0)
}
func (m *MockRepo) HandleEventCanceled(ctx context.Context, tid string, eid uuid.UUID, reason string) error {
	return m.Called(ctx, tid, eid, reason).Error(0)
}

type MockCache struct{ mock.Mock }

func (m *MockCache) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}
func (m *MockCache) Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	return m.Called(ctx, key, val, ttl).Error(0)
}
func (m *MockCache) Del(ctx context.Context, key string) error {
	return m.Called(ctx, key).Error(0)
}

func (m *MockCache) AllowRequest(ctx context.Context, ip string, limit int, window time.Duration) (bool, error) {
	args := m.Called(ctx, ip, limit, window)
	return args.Bool(0), args.Error(1)
}
func (m *MockCache) GetEventCapacity(ctx context.Context, eventID uuid.UUID) (int, error) {
	args := m.Called(ctx, eventID)
	return args.Int(0), args.Error(1)
}
func (m *MockCache) SetEventCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error {
	return m.Called(ctx, eventID, capacity).Error(0)
}

func TestJoinService_Join_Success(t *testing.T) {
	repo := new(MockRepo)
	cache := new(MockCache)
	svc := service.NewJoinService(repo, cache)
	ctx := context.Background()
	eID := uuid.New()
	uID := uuid.New()
	traceID := "trace"

	// Cache miss or error (ignored)
	cache.On("GetEventCapacity", ctx, eID).Return(0, domain.ErrCacheMiss)
	// Repo join
	repo.On("JoinEvent", ctx, traceID, "", eID, uID).Return(domain.StatusActive, nil)

	status, err := svc.Join(ctx, traceID, "", eID, uID)
	assert.NoError(t, err)
	assert.Equal(t, "active", status)
	repo.AssertExpectations(t)
}

/*
TestJoin_Cached removed as JoinService no longer checks user participation status in cache,
only capacity. Capacity caching is tested in TestJoinService_Join_RespectsCacheFastFail.
*/

func TestJoinService_Cancel_Proxies(t *testing.T) {
	repo := new(MockRepo)
	cache := new(MockCache)
	svc := service.NewJoinService(repo, cache)
	ctx := context.Background()
	eID := uuid.New()
	uID := uuid.New()
	traceID := "trace"

	repo.On("CancelJoin", ctx, traceID, "", eID, uID).Return(nil)
	cache.On("Del", mock.Anything).Return(nil)
	// We might need to mock ListWaitlist if Cancel triggers loop, but usually it returns error or we mock it.
	// Check implementation: Cancel -> repo.CancelJoin -> cache.Del -> ListWaitlist (async/background?)
	// Actually implementation says: Cancel -> repo.CancelJoin -> s.cache.Del
	// The waitlist promotion is often DB side or explicit call.
	// In the previous code, I saw ListWaitlist mock Maybe().
	repo.On("ListWaitlist", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, nil).Maybe()

	err := svc.Cancel(ctx, traceID, "", eID, uID)
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestJoinService_Join_EventFull(t *testing.T) {
	repo := new(MockRepo)
	cache := new(MockCache)
	svc := service.NewJoinService(repo, cache)
	ctx := context.Background()
	eID := uuid.New()
	uID := uuid.New()

	cache.On("GetEventCapacity", ctx, eID).Return(0, domain.ErrCacheMiss)
	repo.On("JoinEvent", ctx, "trace", "", eID, uID).Return(domain.JoinStatus(""), domain.ErrEventFull)

	_, err := svc.Join(ctx, "trace", "", eID, uID)
	assert.ErrorIs(t, err, domain.ErrEventFull)
}

func TestJoinService_Join_AlreadyJoined(t *testing.T) {
	repo := new(MockRepo)
	cache := new(MockCache)
	svc := service.NewJoinService(repo, cache)
	ctx := context.Background()
	eID := uuid.New()
	uID := uuid.New()

	cache.On("GetEventCapacity", ctx, eID).Return(0, domain.ErrCacheMiss)
	repo.On("JoinEvent", ctx, "trace", "", eID, uID).Return(domain.StatusActive, domain.ErrAlreadyJoined)

	_, err := svc.Join(ctx, "trace", "", eID, uID)
	assert.ErrorIs(t, err, domain.ErrAlreadyJoined)
}

func TestJoinService_GuardedReads_And_Moderation(t *testing.T) {
	ctx := context.Background()
	eventID := uuid.New()
	ownerID := uuid.New()
	otherID := uuid.New()
	adminID := uuid.New()
	traceID := "trace-guarded"
	cursor := (*domain.KeysetCursor)(nil)

	t.Run("ListParticipants: forbidden for non-owner", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		repo.On("GetEventOwnerID", ctx, eventID).Return(ownerID, nil).Once()

		_, _, err := svc.ListParticipants(ctx, eventID, otherID, "organizer", 10, cursor)
		assert.ErrorIs(t, err, domain.ErrForbidden)
		repo.AssertNotCalled(t, "ListParticipants", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("ListParticipants: owner ok", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		repo.On("GetEventOwnerID", ctx, eventID).Return(ownerID, nil).Once()
		repo.On("ListParticipants", ctx, eventID, 10, cursor).Return([]domain.JoinRecord{}, (*domain.KeysetCursor)(nil), nil).Once()

		_, _, err := svc.ListParticipants(ctx, eventID, ownerID, "organizer", 10, cursor)
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("ListParticipants: admin bypasses owner check", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		repo.On("ListParticipants", ctx, eventID, 10, cursor).Return([]domain.JoinRecord{}, (*domain.KeysetCursor)(nil), nil).Once()

		_, _, err := svc.ListParticipants(ctx, eventID, adminID, "admin", 10, cursor)
		assert.NoError(t, err)
		repo.AssertNotCalled(t, "GetEventOwnerID", mock.Anything, mock.Anything)
	})

	t.Run("ListWaitlist / GetStats share same guard semantics (spot check)", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		repo.On("GetEventOwnerID", ctx, eventID).Return(ownerID, nil).Twice()
		repo.On("ListWaitlist", ctx, eventID, 10, cursor).Return([]domain.JoinRecord{}, (*domain.KeysetCursor)(nil), nil).Once()
		repo.On("GetStats", ctx, eventID).Return(domain.EventStats{Capacity: 1}, nil).Once()

		_, _, err := svc.ListWaitlist(ctx, eventID, ownerID, "organizer", 10, cursor)
		assert.NoError(t, err)

		_, err = svc.GetStats(ctx, eventID, ownerID, "organizer")
		assert.NoError(t, err)

		repo.AssertExpectations(t)
	})

	t.Run("Kick: forbidden for non-owner", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		repo.On("GetEventOwnerID", ctx, eventID).Return(ownerID, nil).Once()

		err := svc.Kick(ctx, traceID, eventID, uuid.New(), otherID, "organizer", "reason")
		assert.ErrorIs(t, err, domain.ErrForbidden)
		repo.AssertNotCalled(t, "Kick", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Ban/Unban: admin bypasses owner check", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		target := uuid.New()
		repo.On("Ban", ctx, traceID, eventID, target, adminID, "reason", (*time.Time)(nil)).Return(nil).Once()
		repo.On("Unban", ctx, traceID, eventID, target, adminID).Return(nil).Once()

		err := svc.Ban(ctx, traceID, eventID, target, adminID, "admin", "reason", nil)
		assert.NoError(t, err)
		err = svc.Unban(ctx, traceID, eventID, target, adminID, "admin")
		assert.NoError(t, err)

		repo.AssertExpectations(t)
		repo.AssertNotCalled(t, "GetEventOwnerID", mock.Anything, mock.Anything)
	})

	t.Run("Owner lookup error is propagated (guard)", func(t *testing.T) {
		repo := new(MockRepo)
		svc := service.NewJoinService(repo, nil)

		boom := errors.New("db down")
		repo.On("GetEventOwnerID", ctx, eventID).Return(uuid.Nil, boom).Once()

		_, _, err := svc.ListParticipants(ctx, eventID, ownerID, "organizer", 10, cursor)
		assert.ErrorIs(t, err, boom)
	})
}
