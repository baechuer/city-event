package media

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	repo            Repository
	storage         Storage
	eventAuthorizer EventAuthorizer
	bucket          string
	now             func() time.Time
}

type UploadIntent struct {
	Asset        Asset             `json:"asset"`
	UploadURL    string            `json:"uploadUrl"`
	UploadMethod string            `json:"uploadMethod"`
	UploadForm   map[string]string `json:"uploadForm,omitempty"`
}

func NewService(repo Repository, storage Storage, bucket string, eventAuthorizer EventAuthorizer) *Service {
	if eventAuthorizer == nil {
		eventAuthorizer = denyEventAuthorizer{}
	}
	return &Service{
		repo:            repo,
		storage:         storage,
		eventAuthorizer: eventAuthorizer,
		bucket:          strings.TrimSpace(bucket),
		now:             time.Now,
	}
}

func (s *Service) CreateUpload(ctx context.Context, cmd UploadCommand) (UploadIntent, error) {
	asset, err := NewAsset(cmd, s.bucket, s.now())
	if err != nil {
		return UploadIntent{}, err
	}
	if err := s.eventAuthorizer.AuthorizeCreateMedia(ctx, EventMediaAuthorization{
		EventID:     asset.EventID,
		UserID:      asset.UploaderID,
		Role:        cmd.UploaderRole,
		AccessToken: cmd.AccessToken,
	}); err != nil {
		return UploadIntent{}, err
	}
	if err := s.storage.EnsureBucket(ctx, s.bucket); err != nil {
		return UploadIntent{}, err
	}
	target, err := s.storage.PresignedUpload(ctx, asset.Bucket, asset.ObjectKey, asset.ContentType, asset.SizeBytes, 15*time.Minute)
	if err != nil {
		return UploadIntent{}, err
	}
	created, err := s.repo.Create(ctx, asset)
	if err != nil {
		return UploadIntent{}, err
	}
	return UploadIntent{Asset: created, UploadURL: target.URL, UploadMethod: target.Method, UploadForm: target.FormData}, nil
}

func (s *Service) MarkUploaded(ctx context.Context, mediaID, userID string) (Asset, error) {
	if strings.TrimSpace(userID) == "" {
		return Asset{}, ErrUnauthorized
	}
	return s.repo.MarkUploaded(ctx, strings.TrimSpace(mediaID), strings.TrimSpace(userID), s.now())
}

func (s *Service) Get(ctx context.Context, mediaID string) (Asset, error) {
	return s.repo.Get(ctx, strings.TrimSpace(mediaID))
}

type Worker struct {
	repo    Repository
	storage Storage
	now     func() time.Time
}

func NewWorker(repo Repository, storage Storage) *Worker {
	return &Worker{repo: repo, storage: storage, now: time.Now}
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	asset, ok, err := w.repo.ClaimNextUploaded(ctx, w.now())
	if err != nil || !ok {
		return ok, err
	}
	info, err := w.storage.StatObject(ctx, asset.Bucket, asset.ObjectKey)
	if err != nil {
		_, _ = w.repo.MarkFailed(ctx, asset.ID, err.Error(), w.now())
		return true, err
	}
	if !info.Exists {
		_, err := w.repo.MarkFailed(ctx, asset.ID, "object missing", w.now())
		return true, err
	}
	if info.SizeBytes != asset.SizeBytes {
		_, err := w.repo.MarkFailed(ctx, asset.ID, "object size does not match upload intent", w.now())
		return true, err
	}
	if strings.ToLower(strings.TrimSpace(info.ContentType)) != asset.ContentType {
		_, err := w.repo.MarkFailed(ctx, asset.ID, "object content type does not match upload intent", w.now())
		return true, err
	}
	_, err = w.repo.MarkReady(ctx, asset.ID, w.now())
	return true, err
}
