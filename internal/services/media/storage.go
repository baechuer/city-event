package media

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage interface {
	EnsureBucket(context.Context, string) error
	PresignedPutURL(context.Context, string, string, time.Duration) (string, error)
	ObjectExists(context.Context, string, string) (bool, error)
}

type MinIOStorage struct {
	client *minio.Client
}

func NewMinIOStorage(endpoint, accessKey, secretKey string) (*MinIOStorage, error) {
	endpoint = strings.TrimSpace(endpoint)
	secure := false
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		secure = parsed.Scheme == "https"
		endpoint = parsed.Host
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, err
	}
	return &MinIOStorage{client: client}, nil
}

func (s *MinIOStorage) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
}

func (s *MinIOStorage) PresignedPutURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error) {
	raw, err := s.client.PresignedPutObject(ctx, bucket, objectKey, expiry)
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *MinIOStorage) ObjectExists(ctx context.Context, bucket, objectKey string) (bool, error) {
	_, err := s.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) && minioErr.Code == "NoSuchKey" {
		return false, nil
	}
	return false, err
}

func (s *MinIOStorage) PutObject(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucket, objectKey, reader, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

type MemoryStorage struct {
	objects map[string]bool
	err     error
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{objects: map[string]bool{}}
}

func (s *MemoryStorage) EnsureBucket(context.Context, string) error {
	return s.err
}

func (s *MemoryStorage) PresignedPutURL(_ context.Context, bucket, objectKey string, _ time.Duration) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return "memory://" + bucket + "/" + objectKey, nil
}

func (s *MemoryStorage) ObjectExists(_ context.Context, bucket, objectKey string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.objects[bucket+"/"+objectKey], nil
}

func (s *MemoryStorage) Put(bucket, objectKey string) {
	s.objects[bucket+"/"+objectKey] = true
}
