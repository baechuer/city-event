package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"

	"github.com/baechuer/cityevents/services/media-service/internal/config"
	"github.com/baechuer/cityevents/services/media-service/internal/domain"
)

// Mocks
type MockRepo struct {
	mock.Mock
}

func (m *MockRepo) Create(ctx context.Context, upload *domain.Upload) error {
	args := m.Called(ctx, upload)
	return args.Error(0)
}
func (m *MockRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Upload, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Upload), args.Error(1)
}
func (m *MockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UploadStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}
func (m *MockRepo) UpdateStatusWithError(ctx context.Context, id uuid.UUID, status domain.UploadStatus, errMsg string) error {
	args := m.Called(ctx, id, status, errMsg)
	return args.Error(0)
}

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) GeneratePresignedPutURL(ctx context.Context, objectKey string, limit int64) (string, error) {
	args := m.Called(ctx, objectKey, limit)
	return args.String(0), args.Error(1)
}
func (m *MockStorage) ObjectExists(ctx context.Context, objectKey string) (bool, int64, error) {
	args := m.Called(ctx, objectKey)
	return args.Bool(0), args.Get(1).(int64), args.Error(2)
}
func (m *MockStorage) DeleteRawObject(ctx context.Context, objectKey string) error {
	args := m.Called(ctx, objectKey)
	return args.Error(0)
}
func (m *MockStorage) PublicURL(objectKey string) string {
	args := m.Called(objectKey)
	return args.String(0)
}

type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) PublishProcessImage(ctx context.Context, uploadID, objectKey, purpose string) error {
	args := m.Called(ctx, uploadID, objectKey, purpose)
	return args.Error(0)
}

func TestRequestUpload_Success(t *testing.T) {
	repo := new(MockRepo)
	s3 := new(MockStorage)
	pub := new(MockPublisher)
	cfg := &config.Config{
		MaxUploadSize: 10 * 1024 * 1024,
		PresignTTL:    5 * time.Minute,
	}
	h := NewUploadHandler(repo, s3, pub, cfg, zerolog.Nop())

	userID := uuid.New()

	// Expectations
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Upload")).Return(nil)
	s3.On("GeneratePresignedPutURL", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/") && strings.HasSuffix(key, ".bin")
	}), int64(10485760)).Return("http://s3/presigned", nil)

	req := httptest.NewRequest("POST", "/request-upload", strings.NewReader(`{"purpose":"avatar"}`))
	req.Header.Set("X-User-ID", userID.String())
	rr := httptest.NewRecorder()

	h.RequestUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	repo.AssertExpectations(t)
	s3.AssertExpectations(t)
}

func TestCompleteUpload_Success(t *testing.T) {
	repo := new(MockRepo)
	s3 := new(MockStorage)
	pub := new(MockPublisher)
	cfg := &config.Config{
		MaxUploadSize: 10 * 1024 * 1024,
	}
	h := NewUploadHandler(repo, s3, pub, cfg, zerolog.Nop())

	userID := uuid.New()
	uploadID := uuid.New()
	objectKey := "raw/" + uploadID.String() + ".bin"

	upload := &domain.Upload{
		ID:           uploadID,
		OwnerID:      userID,
		Purpose:      domain.PurposeAvatar,
		Status:       domain.StatusPending,
		RawObjectKey: objectKey,
	}

	repo.On("GetByID", mock.Anything, uploadID).Return(upload, nil)
	s3.On("ObjectExists", mock.Anything, objectKey).Return(true, int64(1024), nil)
	repo.On("UpdateStatus", mock.Anything, uploadID, domain.StatusUploaded).Return(nil)
	pub.On("PublishProcessImage", mock.Anything, uploadID.String(), objectKey, "avatar").Return(nil)

	body := `{"upload_id":"` + uploadID.String() + `"}`
	req := httptest.NewRequest("POST", "/complete", strings.NewReader(body))
	req.Header.Set("X-User-ID", userID.String())
	rr := httptest.NewRecorder()

	h.CompleteUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	repo.AssertExpectations(t)
	s3.AssertExpectations(t)
	pub.AssertExpectations(t)
}
