package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/baechuer/cityevents/services/media-service/internal/domain"
)

// UploadRepository defines database operations for uploads.
type UploadRepository interface {
	Create(ctx context.Context, upload *domain.Upload) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Upload, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UploadStatus) error
	UpdateStatusWithError(ctx context.Context, id uuid.UUID, status domain.UploadStatus, errMsg string) error
}

// FileStorage defines file operations.
type FileStorage interface {
	GeneratePresignedPutURL(ctx context.Context, objectKey string, contentLengthLimit int64) (string, error)
	ObjectExists(ctx context.Context, objectKey string) (bool, int64, error)
	DeleteRawObject(ctx context.Context, objectKey string) error
	PublicURL(objectKey string) string
}

// MessagePublisher defines message publishing operations.
type MessagePublisher interface {
	PublishProcessImage(ctx context.Context, uploadID, objectKey, purpose string) error
}
