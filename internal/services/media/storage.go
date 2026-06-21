package media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage interface {
	EnsureBucket(context.Context, string) error
	PresignedUpload(context.Context, string, string, string, int64, time.Duration) (UploadTarget, error)
	StatObject(context.Context, string, string) (ObjectInfo, error)
}

type UploadTarget struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	FormData map[string]string `json:"formData,omitempty"`
}

type ObjectInfo struct {
	Exists      bool
	SizeBytes   int64
	ContentType string
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

func (s *MinIOStorage) PresignedUpload(ctx context.Context, bucket, objectKey, contentType string, sizeBytes int64, expiry time.Duration) (UploadTarget, error) {
	policy := minio.NewPostPolicy()
	for _, step := range []func() error{
		func() error { return policy.SetBucket(bucket) },
		func() error { return policy.SetKey(objectKey) },
		func() error { return policy.SetExpires(time.Now().UTC().Add(expiry)) },
		func() error { return policy.SetContentType(contentType) },
		func() error { return policy.SetContentLengthRange(1, sizeBytes) },
	} {
		if err := step(); err != nil {
			return UploadTarget{}, err
		}
	}
	raw, formData, err := s.client.PresignedPostPolicy(ctx, policy)
	if err != nil {
		return UploadTarget{}, err
	}
	return UploadTarget{URL: raw.String(), Method: http.MethodPost, FormData: formData}, nil
}

func (s *MinIOStorage) StatObject(ctx context.Context, bucket, objectKey string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
	if err == nil {
		return ObjectInfo{
			Exists:      true,
			SizeBytes:   info.Size,
			ContentType: strings.ToLower(strings.TrimSpace(info.ContentType)),
		}, nil
	}
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) && minioErr.Code == "NoSuchKey" {
		return ObjectInfo{}, nil
	}
	return ObjectInfo{}, err
}

func (s *MinIOStorage) PutObject(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucket, objectKey, reader, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

type MemoryStorage struct {
	objects map[string]ObjectInfo
	err     error
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{objects: map[string]ObjectInfo{}}
}

func (s *MemoryStorage) EnsureBucket(context.Context, string) error {
	return s.err
}

func (s *MemoryStorage) PresignedUpload(_ context.Context, bucket, objectKey, contentType string, sizeBytes int64, _ time.Duration) (UploadTarget, error) {
	if s.err != nil {
		return UploadTarget{}, s.err
	}
	return UploadTarget{
		URL:    "memory://" + bucket + "/" + objectKey,
		Method: http.MethodPost,
		FormData: map[string]string{
			"key":          objectKey,
			"Content-Type": contentType,
			"maxSizeBytes": strconv.FormatInt(sizeBytes, 10),
		},
	}, nil
}

func (s *MemoryStorage) StatObject(_ context.Context, bucket, objectKey string) (ObjectInfo, error) {
	if s.err != nil {
		return ObjectInfo{}, s.err
	}
	info, ok := s.objects[bucket+"/"+objectKey]
	if !ok {
		return ObjectInfo{}, nil
	}
	info.Exists = true
	info.ContentType = strings.ToLower(strings.TrimSpace(info.ContentType))
	return info, nil
}

func (s *MemoryStorage) Put(bucket, objectKey string) {
	s.PutObjectInfo(bucket, objectKey, 1, "image/jpeg")
}

func (s *MemoryStorage) PutObjectInfo(bucket, objectKey string, sizeBytes int64, contentType string) {
	s.objects[bucket+"/"+objectKey] = ObjectInfo{
		Exists:      true,
		SizeBytes:   sizeBytes,
		ContentType: strings.ToLower(strings.TrimSpace(contentType)),
	}
}
