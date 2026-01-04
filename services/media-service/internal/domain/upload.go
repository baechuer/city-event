package domain

import (
	"time"

	"github.com/google/uuid"
)

// UploadStatus represents the state of an upload.
type UploadStatus string

const (
	StatusPending    UploadStatus = "PENDING"
	StatusUploaded   UploadStatus = "UPLOADED"
	StatusProcessing UploadStatus = "PROCESSING"
	StatusReady      UploadStatus = "READY"
	StatusFailed     UploadStatus = "FAILED"
)

// UploadPurpose indicates what the image is for.
type UploadPurpose string

const (
	PurposeAvatar     UploadPurpose = "avatar"
	PurposeEventCover UploadPurpose = "event_cover"
)

// Upload represents an image upload record.
type Upload struct {
	ID           uuid.UUID         `json:"id"`
	OwnerID      uuid.UUID         `json:"owner_id"`
	Purpose      UploadPurpose     `json:"purpose"`
	Status       UploadStatus      `json:"status"`
	RawObjectKey string            `json:"-"` // never exposed
	DerivedKeys  map[string]string `json:"derived_keys,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// IsTerminal returns true if the upload is in a terminal state.
func (u *Upload) IsTerminal() bool {
	return u.Status == StatusReady || u.Status == StatusFailed
}

// DerivedSizes defines output sizes for each purpose.
var DerivedSizes = map[UploadPurpose][]ImageSize{
	PurposeAvatar: {
		{Name: "256", Width: 256, Height: 256, Crop: true},
		{Name: "512", Width: 512, Height: 512, Crop: true},
	},
	PurposeEventCover: {
		{Name: "800", Width: 800, Height: 0, Crop: false}, // max width, preserve aspect
		{Name: "1600", Width: 1600, Height: 0, Crop: false},
	},
}

// ImageSize defines output image dimensions.
type ImageSize struct {
	Name   string // e.g., "256", "512", "800"
	Width  int
	Height int  // 0 = preserve aspect ratio
	Crop   bool // true = center crop to exact dimensions
}
