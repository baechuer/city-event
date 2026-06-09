package media

import (
	"errors"
	"testing"
	"time"
)

func TestValidateUpload(t *testing.T) {
	valid := UploadCommand{
		EventID:     "event-1",
		UploaderID:  "user-1",
		Filename:    "banner.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   1024,
	}
	if err := ValidateUpload(valid); err != nil {
		t.Fatalf("valid upload: %v", err)
	}
	cases := []struct {
		name string
		cmd  UploadCommand
		want error
	}{
		{"missing user", UploadCommand{EventID: "event-1", Filename: "a.jpg", ContentType: "image/jpeg", SizeBytes: 1}, ErrUnauthorized},
		{"missing event", UploadCommand{UploaderID: "user-1", Filename: "a.jpg", ContentType: "image/jpeg", SizeBytes: 1}, ErrInvalidUpload},
		{"missing filename", UploadCommand{EventID: "event-1", UploaderID: "user-1", ContentType: "image/jpeg", SizeBytes: 1}, ErrInvalidUpload},
		{"bad content type", UploadCommand{EventID: "event-1", UploaderID: "user-1", Filename: "a.gif", ContentType: "image/gif", SizeBytes: 1}, ErrInvalidContentType},
		{"zero size", UploadCommand{EventID: "event-1", UploaderID: "user-1", Filename: "a.jpg", ContentType: "image/jpeg"}, ErrInvalidUpload},
		{"too large", UploadCommand{EventID: "event-1", UploaderID: "user-1", Filename: "a.jpg", ContentType: "image/jpeg", SizeBytes: MaxUploadSizeBytes + 1}, ErrInvalidUpload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateUpload(tc.cmd); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewAssetAndStateTransitions(t *testing.T) {
	asset, err := NewAsset(UploadCommand{
		EventID:     "event-1",
		UploaderID:  "user-1",
		Filename:    "../bad banner.jpg",
		ContentType: "IMAGE/PNG",
		SizeBytes:   1024,
	}, "bucket", time.Now())
	if err != nil {
		t.Fatalf("new asset: %v", err)
	}
	if asset.Status != StatusUploading {
		t.Fatalf("status = %s", asset.Status)
	}
	if asset.ContentType != "image/png" {
		t.Fatalf("content type = %s", asset.ContentType)
	}
	if asset.ObjectKey == "" || asset.ObjectKey == "../bad banner.jpg" {
		t.Fatalf("object key not sanitized: %s", asset.ObjectKey)
	}

	for _, transition := range [][2]string{
		{StatusUploading, StatusUploaded},
		{StatusUploaded, StatusProcessing},
		{StatusProcessing, StatusReady},
		{StatusProcessing, StatusFailed},
	} {
		if err := ValidateTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("transition %s -> %s: %v", transition[0], transition[1], err)
		}
	}
	if err := ValidateTransition(StatusUploading, StatusReady); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("invalid transition error = %v", err)
	}
}
