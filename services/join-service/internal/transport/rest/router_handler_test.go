package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/security"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/service"
	"github.com/baechuer/real-time-ressys/services/join-service/internal/transport/rest/response"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeVerifier struct {
	claims security.TokenClaims
	err    error
}

func (f fakeVerifier) VerifyAccessToken(token string) (security.TokenClaims, error) {
	return f.claims, f.err
}

type fakeCache struct {
	allow bool
	caps  map[uuid.UUID]int
}

func newFakeCache() *fakeCache {
	return &fakeCache{allow: true, caps: map[uuid.UUID]int{}}
}

func (c *fakeCache) GetEventCapacity(ctx context.Context, eventID uuid.UUID) (int, error) {
	v, ok := c.caps[eventID]
	if !ok {
		return 0, domain.ErrCacheMiss
	}
	return v, nil
}

func (c *fakeCache) SetEventCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error {
	c.caps[eventID] = capacity
	return nil
}

func (c *fakeCache) AllowRequest(ctx context.Context, ip string, limit int, window time.Duration) (bool, error) {
	return c.allow, nil
}

type fakeRepo struct {
	joinFn           func(ctx context.Context, traceID string, eventID, userID uuid.UUID) (domain.JoinStatus, error)
	cancelFn         func(ctx context.Context, traceID string, eventID, userID uuid.UUID) error
	listMyFn         func(ctx context.Context, userID uuid.UUID, statuses []domain.JoinStatus, from, to *time.Time, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error)
	listParticipants func(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error)
	listWaitlist     func(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error)
	getStatsFn       func(ctx context.Context, eventID uuid.UUID) (domain.EventStats, error)

	kickFn     func(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string) error
	banFn      func(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string, expiresAt *time.Time) error
	unbanFn    func(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID) error
	ownerFn    func(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error)
	notImplErr error
}

func (r *fakeRepo) notImpl() error {
	if r.notImplErr != nil {
		return r.notImplErr
	}
	return errors.New("not implemented")
}

// --- domain.JoinRepository ---

func (r *fakeRepo) JoinEvent(ctx context.Context, traceID string, eventID, userID uuid.UUID) (domain.JoinStatus, error) {
	if r.joinFn == nil {
		return "", r.notImpl()
	}
	return r.joinFn(ctx, traceID, eventID, userID)
}

func (r *fakeRepo) CancelJoin(ctx context.Context, traceID string, eventID, userID uuid.UUID) error {
	if r.cancelFn == nil {
		return r.notImpl()
	}
	return r.cancelFn(ctx, traceID, eventID, userID)
}

func (r *fakeRepo) HandleEventCanceled(ctx context.Context, traceID string, eventID uuid.UUID, reason string) error {
	return r.notImpl()
}

func (r *fakeRepo) InitCapacity(ctx context.Context, eventID uuid.UUID, capacity int) error {
	return r.notImpl()
}

func (f *fakeRepo) GetByEventAndUser(ctx context.Context, eventID, userID uuid.UUID) (domain.JoinRecord, error) {
	// Mock behavior: if eventID == "0000...0000" return ErrNotJoined, else return Active
	if eventID == uuid.Nil {
		return domain.JoinRecord{}, domain.ErrNotJoined
	}
	return domain.JoinRecord{
		ID:        uuid.New(),
		EventID:   eventID,
		UserID:    userID,
		Status:    domain.StatusActive,
		CreatedAt: time.Now(),
	}, nil
}

func (r *fakeRepo) ListMyJoins(ctx context.Context, userID uuid.UUID, statuses []domain.JoinStatus, from, to *time.Time, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	if r.listMyFn == nil {
		return nil, nil, r.notImpl()
	}
	return r.listMyFn(ctx, userID, statuses, from, to, limit, cursor)
}

func (r *fakeRepo) ListParticipants(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	if r.listParticipants == nil {
		return nil, nil, r.notImpl()
	}
	return r.listParticipants(ctx, eventID, limit, cursor)
}

func (r *fakeRepo) ListWaitlist(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	if r.listWaitlist == nil {
		return nil, nil, r.notImpl()
	}
	return r.listWaitlist(ctx, eventID, limit, cursor)
}

func (r *fakeRepo) GetStats(ctx context.Context, eventID uuid.UUID) (domain.EventStats, error) {
	if r.getStatsFn == nil {
		return domain.EventStats{}, r.notImpl()
	}
	return r.getStatsFn(ctx, eventID)
}

func (r *fakeRepo) Kick(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string) error {
	if r.kickFn == nil {
		return r.notImpl()
	}
	return r.kickFn(ctx, traceID, eventID, targetUserID, actorID, reason)
}

func (r *fakeRepo) Ban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, reason string, expiresAt *time.Time) error {
	if r.banFn == nil {
		return r.notImpl()
	}
	return r.banFn(ctx, traceID, eventID, targetUserID, actorID, reason, expiresAt)
}

func (r *fakeRepo) Unban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID) error {
	if r.unbanFn == nil {
		return r.notImpl()
	}
	return r.unbanFn(ctx, traceID, eventID, targetUserID, actorID)
}

func (r *fakeRepo) GetEventOwnerID(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error) {
	if r.ownerFn == nil {
		return uuid.Nil, r.notImpl()
	}
	return r.ownerFn(ctx, eventID)
}

func newTestRouter(repo domain.JoinRepository, cache domain.CacheRepository, claims security.TokenClaims) http.Handler {
	svc := service.NewJoinService(repo, cache)
	h := NewHandler(svc)
	return NewRouter(RouterDeps{
		Cache:     cache,
		Handler:   h,
		Verifier:  fakeVerifier{claims: claims},
		JWTIssuer: claims.Issuer,
	})
}

func decodeData(t *testing.T, rr *httptest.ResponseRecorder) response.Envelope {
	t.Helper()
	var env response.Envelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &env))
	return env
}

func decodeError(t *testing.T, rr *httptest.ResponseRecorder) response.ErrorBody {
	t.Helper()
	var errBody response.ErrorBody
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errBody))
	return errBody
}

func TestNewRouter_PanicsOnNilDeps(t *testing.T) {
	cache := newFakeCache()
	repo := &fakeRepo{}
	svc := service.NewJoinService(repo, cache)
	h := NewHandler(svc)

	require.Panics(t, func() {
		_ = NewRouter(RouterDeps{Cache: nil, Handler: h, Verifier: fakeVerifier{}, JWTIssuer: "x"})
	})
	require.Panics(t, func() {
		_ = NewRouter(RouterDeps{Cache: cache, Handler: nil, Verifier: fakeVerifier{}, JWTIssuer: "x"})
	})
	require.Panics(t, func() {
		_ = NewRouter(RouterDeps{Cache: cache, Handler: h, Verifier: nil, JWTIssuer: "x"})
	})
}

func TestRouter_Join_InvalidJSON_400(t *testing.T) {
	cache := newFakeCache()
	repo := &fakeRepo{
		joinFn: func(ctx context.Context, traceID string, eventID, userID uuid.UUID) (domain.JoinStatus, error) {
			return domain.StatusActive, nil
		},
	}
	uid := uuid.New()
	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/join", bytes.NewBufferString("{bad"))
	req.Header.Set("Authorization", "Bearer ok")
	req.Header.Set("X-Request-Id", "rid-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	errBody := decodeError(t, rr)
	require.Equal(t, "request.invalid", errBody.Error.Code)
	require.Equal(t, "rid-1", errBody.Error.RequestID)
}

func TestRouter_Join_InvalidEventID_400(t *testing.T) {
	cache := newFakeCache()
	repo := &fakeRepo{
		joinFn: func(ctx context.Context, traceID string, eventID, userID uuid.UUID) (domain.JoinStatus, error) {
			return domain.StatusActive, nil
		},
	}
	uid := uuid.New()
	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	body := `{"event_id":"not-a-uuid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/join", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	errBody := decodeError(t, rr)
	require.Equal(t, "request.invalid", errBody.Error.Code)
	require.Contains(t, errBody.Error.Message, "event_id")
}

func TestRouter_Join_Success_200(t *testing.T) {
	cache := newFakeCache()
	ev := uuid.New()
	uid := uuid.New()

	repo := &fakeRepo{
		joinFn: func(ctx context.Context, traceID string, eventID, userID uuid.UUID) (domain.JoinStatus, error) {
			require.Equal(t, ev, eventID)
			require.Equal(t, uid, userID)
			return domain.StatusWaitlisted, nil
		},
	}

	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	body := `{"event_id":"` + ev.String() + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/join", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	env := decodeData(t, rr)
	m := env.Data.(map[string]any)
	require.Equal(t, "waitlisted", m["status"])
}

func TestRouter_Join_EventFull_409(t *testing.T) {
	cache := newFakeCache()
	ev := uuid.New()
	uid := uuid.New()

	repo := &fakeRepo{
		joinFn: func(ctx context.Context, traceID string, eventID, userID uuid.UUID) (domain.JoinStatus, error) {
			return "", domain.ErrEventFull
		},
	}

	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	body := `{"event_id":"` + ev.String() + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/join", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	errBody := decodeError(t, rr)
	require.Equal(t, "event.full", errBody.Error.Code)
}

func TestRouter_Cancel_InvalidEventID_400(t *testing.T) {
	cache := newFakeCache()
	repo := &fakeRepo{}
	uid := uuid.New()

	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/join/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	errBody := decodeError(t, rr)
	require.Equal(t, "request.invalid", errBody.Error.Code)
}

func TestRouter_MeJoins_InvalidCursor_IsIgnored_200(t *testing.T) {
	cache := newFakeCache()

	var gotCursor *domain.KeysetCursor
	repo := &fakeRepo{
		listMyFn: func(ctx context.Context, userID uuid.UUID, statuses []domain.JoinStatus, from, to *time.Time, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
			gotCursor = cursor
			return []domain.JoinRecord{}, nil, nil
		},
	}

	uid := uuid.New()
	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/joins?cursor=%%%bad", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Nil(t, gotCursor, "invalid cursor should be ignored and treated as nil")
}

func TestRouter_Reads_OrganizerGuard_ForbiddenForNonOwner(t *testing.T) {
	cache := newFakeCache()
	ev := uuid.New()
	uid := uuid.New()
	owner := uuid.New() // different => forbidden

	repo := &fakeRepo{
		ownerFn: func(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error) {
			return owner, nil
		},
		listParticipants: func(ctx context.Context, eventID uuid.UUID, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
			return nil, nil, nil
		},
	}

	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+ev.String()+"/participants", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	errBody := decodeError(t, rr)
	require.Equal(t, "auth.forbidden", errBody.Error.Code)
}

func TestRouter_RateLimit_429(t *testing.T) {
	cache := newFakeCache()
	cache.allow = false

	repo := &fakeRepo{}
	uid := uuid.New()
	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/joins", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusTooManyRequests, rr.Code)
}

func TestRouter_SecurityHeaders_PresentOnOK(t *testing.T) {
	cache := newFakeCache()
	repo := &fakeRepo{
		listMyFn: func(ctx context.Context, userID uuid.UUID, statuses []domain.JoinStatus, from, to *time.Time, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
			return []domain.JoinRecord{}, nil, nil
		},
	}
	uid := uuid.New()
	r := newTestRouter(repo, cache, security.TokenClaims{
		UserID: uid.String(),
		Role:   "user",
		Issuer: "auth-service",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/joins", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	require.Contains(t, rr.Header().Get("Content-Security-Policy"), "default-src")
}
