//go:build integration

package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresMediaRepositoryAndMinIOReadyFlow(t *testing.T) {
	ctx := context.Background()
	pool := setupMediaPostgres(t, ctx)
	storage := setupMediaStorage(t, ctx)
	service := NewService(NewPostgresRepository(pool), storage, testMediaBucket(), allowEventAuthorizer{})
	worker := NewWorker(NewPostgresRepository(pool), storage)

	intent := createMediaIntent(t, ctx, service, "event-1", "user-1", "banner.jpg")
	if err := storage.PutObject(ctx, intent.Asset.Bucket, intent.Asset.ObjectKey, bytes.NewReader([]byte("image-bytes")), int64(len("image-bytes")), intent.Asset.ContentType); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if _, err := service.MarkUploaded(ctx, intent.Asset.ID, "user-1"); err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}
	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process asset")
	}
	asset, err := service.Get(ctx, intent.Asset.ID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if asset.Status != StatusReady || asset.ProcessedAt == nil {
		t.Fatalf("asset = %+v, want ready", asset)
	}
}

func TestPostgresMediaWorkerMarksMissingObjectFailed(t *testing.T) {
	ctx := context.Background()
	pool := setupMediaPostgres(t, ctx)
	storage := setupMediaStorage(t, ctx)
	service := NewService(NewPostgresRepository(pool), storage, testMediaBucket(), allowEventAuthorizer{})
	worker := NewWorker(NewPostgresRepository(pool), storage)

	intent := createMediaIntent(t, ctx, service, "event-1", "user-1", "missing.jpg")
	if _, err := service.MarkUploaded(ctx, intent.Asset.ID, "user-1"); err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}
	if _, err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process missing: %v", err)
	}
	asset, err := service.Get(ctx, intent.Asset.ID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if asset.Status != StatusFailed || asset.FailureReason == "" {
		t.Fatalf("asset = %+v, want failed", asset)
	}
}

func TestPostgresMediaWorkerProcessesFiftyAssets(t *testing.T) {
	ctx := context.Background()
	pool := setupMediaPostgres(t, ctx)
	storage := setupMediaStorage(t, ctx)
	repo := NewPostgresRepository(pool)
	service := NewService(repo, storage, testMediaBucket(), allowEventAuthorizer{})
	worker := NewWorker(repo, storage)

	for i := 0; i < 50; i++ {
		intent := createMediaIntent(t, ctx, service, fmt.Sprintf("event-%02d", i), "user-1", fmt.Sprintf("banner-%02d.jpg", i))
		if err := storage.PutObject(ctx, intent.Asset.Bucket, intent.Asset.ObjectKey, bytes.NewReader([]byte("image-bytes")), int64(len("image-bytes")), intent.Asset.ContentType); err != nil {
			t.Fatalf("put object %d: %v", i, err)
		}
		if _, err := service.MarkUploaded(ctx, intent.Asset.ID, "user-1"); err != nil {
			t.Fatalf("mark uploaded %d: %v", i, err)
		}
	}
	for i := 0; i < 50; i++ {
		processed, err := worker.ProcessOne(ctx)
		if err != nil {
			t.Fatalf("process %d: %v", i, err)
		}
		if !processed {
			t.Fatalf("process %d returned false", i)
		}
	}
	ready, err := repo.CountByStatus(ctx, StatusReady)
	if err != nil {
		t.Fatalf("ready count: %v", err)
	}
	if ready != 50 {
		t.Fatalf("ready count = %d, want 50", ready)
	}
}

func TestPostgresMediaConcurrentWorkerClaim(t *testing.T) {
	ctx := context.Background()
	pool := setupMediaPostgres(t, ctx)
	storage := setupMediaStorage(t, ctx)
	repo := NewPostgresRepository(pool)
	service := NewService(repo, storage, testMediaBucket(), allowEventAuthorizer{})
	intent := createMediaIntent(t, ctx, service, "event-1", "user-1", "banner.jpg")
	if err := storage.PutObject(ctx, intent.Asset.Bucket, intent.Asset.ObjectKey, bytes.NewReader([]byte("image-bytes")), int64(len("image-bytes")), intent.Asset.ContentType); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if _, err := service.MarkUploaded(ctx, intent.Asset.ID, "user-1"); err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processed, err := NewWorker(repo, storage).ProcessOne(ctx)
			results <- processed
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("worker error: %v", err)
		}
	}
	processedCount := 0
	for processed := range results {
		if processed {
			processedCount++
		}
	}
	if processedCount != 1 {
		t.Fatalf("processed count = %d, want 1", processedCount)
	}
	ready, err := repo.CountByStatus(ctx, StatusReady)
	if err != nil {
		t.Fatalf("ready count: %v", err)
	}
	if ready != 1 {
		t.Fatalf("ready count = %d, want 1", ready)
	}
}

func setupMediaPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PHASE7_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_lock(424242)`); err != nil {
		t.Fatalf("take integration schema lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock(424242)`)
	})
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS media_assets;`); err != nil {
		t.Fatalf("reset media tables: %v", err)
	}
	path := filepath.Join("..", "..", "..", "migrations", "media", "001_init.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return pool
}

func setupMediaStorage(t *testing.T, ctx context.Context) *MinIOStorage {
	t.Helper()
	endpoint := os.Getenv("PHASE7_TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	accessKey := os.Getenv("PHASE7_TEST_MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "cityevents"
	}
	secretKey := os.Getenv("PHASE7_TEST_MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "cityevents-password"
	}
	storage, err := NewMinIOStorage(endpoint, accessKey, secretKey)
	if err != nil {
		t.Fatalf("new minio storage: %v", err)
	}
	if err := storage.EnsureBucket(ctx, testMediaBucket()); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	return storage
}

func testMediaBucket() string {
	value := os.Getenv("PHASE7_TEST_MINIO_BUCKET")
	if value == "" {
		return "cityevents-media-test"
	}
	return value
}

func createMediaIntent(t *testing.T, ctx context.Context, service *Service, eventID, uploaderID, filename string) UploadIntent {
	t.Helper()
	intent, err := service.CreateUpload(ctx, UploadCommand{
		EventID:     eventID,
		UploaderID:  uploaderID,
		Filename:    filename,
		ContentType: "image/jpeg",
		SizeBytes:   int64(len("image-bytes")),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	return intent
}
