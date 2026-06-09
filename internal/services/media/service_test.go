package media

import (
	"context"
	"testing"
	"time"
)

func TestServiceCreateUploadMarkUploadedAndWorkerReady(t *testing.T) {
	repo := NewMemoryRepository()
	storage := NewMemoryStorage()
	service := NewService(repo, storage, "bucket")
	service.now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }

	intent, err := service.CreateUpload(context.Background(), UploadCommand{
		EventID:     "event-1",
		UploaderID:  "user-1",
		Filename:    "banner.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   10,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if intent.UploadURL == "" || intent.Asset.Status != StatusUploading {
		t.Fatalf("intent = %+v", intent)
	}
	asset, err := service.MarkUploaded(context.Background(), intent.Asset.ID, "user-1")
	if err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}
	storage.Put(asset.Bucket, asset.ObjectKey)
	worker := NewWorker(repo, storage)
	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one asset")
	}
	ready, err := service.Get(context.Background(), asset.ID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if ready.Status != StatusReady {
		t.Fatalf("status = %s, want ready", ready.Status)
	}
}

func TestWorkerMarksMissingObjectFailed(t *testing.T) {
	repo := NewMemoryRepository()
	storage := NewMemoryStorage()
	service := NewService(repo, storage, "bucket")
	intent, err := service.CreateUpload(context.Background(), UploadCommand{
		EventID:     "event-1",
		UploaderID:  "user-1",
		Filename:    "banner.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   10,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if _, err := service.MarkUploaded(context.Background(), intent.Asset.ID, "user-1"); err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}
	if _, err := NewWorker(repo, storage).ProcessOne(context.Background()); err != nil {
		t.Fatalf("process missing object: %v", err)
	}
	asset, err := service.Get(context.Background(), intent.Asset.ID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if asset.Status != StatusFailed || asset.FailureReason == "" {
		t.Fatalf("asset = %+v, want failed", asset)
	}
}
