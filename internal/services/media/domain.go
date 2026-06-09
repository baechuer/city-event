package media

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	StatusUploading  = "UPLOADING"
	StatusUploaded   = "UPLOADED"
	StatusProcessing = "PROCESSING"
	StatusReady      = "READY"
	StatusFailed     = "FAILED"

	MaxUploadSizeBytes = 10 * 1024 * 1024
)

var (
	ErrUnauthorized           = errors.New("user identity required")
	ErrForbidden              = errors.New("forbidden")
	ErrNotFound               = errors.New("not found")
	ErrInvalidUpload          = errors.New("invalid upload")
	ErrInvalidContentType     = errors.New("unsupported content type")
	ErrInvalidStateTransition = errors.New("invalid state transition")
)

type Asset struct {
	ID            string     `json:"id"`
	EventID       string     `json:"eventId"`
	UploaderID    string     `json:"uploaderId"`
	Filename      string     `json:"filename"`
	ContentType   string     `json:"contentType"`
	SizeBytes     int64      `json:"sizeBytes"`
	Bucket        string     `json:"bucket"`
	ObjectKey     string     `json:"objectKey"`
	Status        string     `json:"status"`
	FailureReason string     `json:"failureReason,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	ProcessedAt   *time.Time `json:"processedAt,omitempty"`
}

type UploadCommand struct {
	EventID     string
	UploaderID  string
	Filename    string
	ContentType string
	SizeBytes   int64
}

func NewAsset(cmd UploadCommand, bucket string, now time.Time) (Asset, error) {
	cmd.EventID = strings.TrimSpace(cmd.EventID)
	cmd.UploaderID = strings.TrimSpace(cmd.UploaderID)
	cmd.Filename = strings.TrimSpace(cmd.Filename)
	cmd.ContentType = strings.TrimSpace(strings.ToLower(cmd.ContentType))
	if err := ValidateUpload(cmd); err != nil {
		return Asset{}, err
	}
	id := NewID()
	return Asset{
		ID:          id,
		EventID:     cmd.EventID,
		UploaderID:  cmd.UploaderID,
		Filename:    cmd.Filename,
		ContentType: cmd.ContentType,
		SizeBytes:   cmd.SizeBytes,
		Bucket:      strings.TrimSpace(bucket),
		ObjectKey:   objectKey(cmd.EventID, id, cmd.Filename),
		Status:      StatusUploading,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}, nil
}

func ValidateUpload(cmd UploadCommand) error {
	if strings.TrimSpace(cmd.UploaderID) == "" {
		return ErrUnauthorized
	}
	if strings.TrimSpace(cmd.EventID) == "" || strings.TrimSpace(cmd.Filename) == "" {
		return ErrInvalidUpload
	}
	if !AllowedContentType(cmd.ContentType) {
		return ErrInvalidContentType
	}
	if cmd.SizeBytes <= 0 || cmd.SizeBytes > MaxUploadSizeBytes {
		return ErrInvalidUpload
	}
	return nil
}

func AllowedContentType(contentType string) bool {
	switch strings.TrimSpace(strings.ToLower(contentType)) {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func ValidateTransition(from, to string) error {
	allowed := map[string][]string{
		StatusUploading:  {StatusUploaded, StatusFailed},
		StatusUploaded:   {StatusProcessing, StatusFailed},
		StatusProcessing: {StatusReady, StatusFailed},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return nil
		}
	}
	return ErrInvalidStateTransition
}

var unsafeFilename = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func objectKey(eventID, mediaID, filename string) string {
	name := filepath.Base(filename)
	name = unsafeFilename.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-")
	if name == "" {
		name = "media"
	}
	return "events/" + eventID + "/" + mediaID + "/" + name
}

func NewID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
