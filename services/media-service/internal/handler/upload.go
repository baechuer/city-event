package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/baechuer/cityevents/services/media-service/internal/config"
	"github.com/baechuer/cityevents/services/media-service/internal/domain"
)

// UploadHandler handles upload-related HTTP requests.
type UploadHandler struct {
	repo      UploadRepository
	s3        FileStorage
	publisher MessagePublisher
	cfg       *config.Config
	log       zerolog.Logger
}

// NewUploadHandler creates a new upload handler.
func NewUploadHandler(
	repo UploadRepository,
	s3 FileStorage,
	publisher MessagePublisher,
	cfg *config.Config,
	log zerolog.Logger,
) *UploadHandler {
	return &UploadHandler{
		repo:      repo,
		s3:        s3,
		publisher: publisher,
		cfg:       cfg,
		log:       log,
	}
}

// RequestUploadRequest is the request body for requesting an upload URL.
type RequestUploadRequest struct {
	Purpose string `json:"purpose"` // "avatar" or "event_cover"
}

// RequestUploadResponse is the response for a presigned upload URL.
type RequestUploadResponse struct {
	UploadID     string `json:"upload_id"`
	PresignedURL string `json:"presigned_url"`
	ObjectKey    string `json:"object_key"`
	ExpiresAt    string `json:"expires_at"`
}

// RequestUpload creates a new upload record and returns a presigned PUT URL.
func (h *UploadHandler) RequestUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get owner ID from header (set by BFF after auth)
	ownerIDStr := r.Header.Get("X-User-ID")
	if ownerIDStr == "" {
		h.errorResponse(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	ownerID, err := uuid.Parse(ownerIDStr)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var req RequestUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	purpose := domain.UploadPurpose(req.Purpose)
	if purpose != domain.PurposeAvatar && purpose != domain.PurposeEventCover {
		h.errorResponse(w, http.StatusBadRequest, "invalid purpose")
		return
	}

	// Create upload record
	uploadID := uuid.New()
	objectKey := "raw/" + uploadID.String() + ".bin"
	now := time.Now()

	upload := &domain.Upload{
		ID:           uploadID,
		OwnerID:      ownerID,
		Purpose:      purpose,
		Status:       domain.StatusPending,
		RawObjectKey: objectKey,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.repo.Create(ctx, upload); err != nil {
		h.log.Error().Err(err).Msg("failed to create upload record")
		h.errorResponse(w, http.StatusInternalServerError, "failed to create upload")
		return
	}

	// Generate presigned URL
	presignedURL, err := h.s3.GeneratePresignedPutURL(ctx, objectKey, h.cfg.MaxUploadSize)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to generate presigned URL")
		h.errorResponse(w, http.StatusInternalServerError, "failed to generate upload URL")
		return
	}

	resp := RequestUploadResponse{
		UploadID:     uploadID.String(),
		PresignedURL: presignedURL,
		ObjectKey:    objectKey,
		ExpiresAt:    now.Add(h.cfg.PresignTTL).Format(time.RFC3339),
	}

	h.jsonResponse(w, http.StatusOK, resp)
}

// CompleteUploadRequest is the request body for completing an upload.
type CompleteUploadRequest struct {
	UploadID string `json:"upload_id"`
}

// CompleteUploadResponse is the response after marking upload complete.
type CompleteUploadResponse struct {
	Status string `json:"status"`
}

// CompleteUpload marks an upload as uploaded and queues it for processing.
func (h *UploadHandler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get owner ID from header
	ownerIDStr := r.Header.Get("X-User-ID")
	if ownerIDStr == "" {
		h.errorResponse(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	ownerID, err := uuid.Parse(ownerIDStr)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var req CompleteUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	uploadID, err := uuid.Parse(req.UploadID)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid upload ID")
		return
	}

	// Get upload record
	upload, err := h.repo.GetByID(ctx, uploadID)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to get upload")
		h.errorResponse(w, http.StatusInternalServerError, "failed to get upload")
		return
	}
	if upload == nil {
		h.errorResponse(w, http.StatusNotFound, "upload not found")
		return
	}

	// Verify ownership
	if upload.OwnerID != ownerID {
		h.errorResponse(w, http.StatusForbidden, "not authorized")
		return
	}

	// Idempotency: if already processed, just return current status
	if upload.Status != domain.StatusPending {
		h.jsonResponse(w, http.StatusOK, CompleteUploadResponse{Status: string(upload.Status)})
		return
	}

	// Verify object exists in S3
	exists, size, err := h.s3.ObjectExists(ctx, upload.RawObjectKey)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to check object existence")
		h.errorResponse(w, http.StatusInternalServerError, "failed to verify upload")
		return
	}
	if !exists {
		h.errorResponse(w, http.StatusBadRequest, "file not uploaded yet")
		return
	}

	// Validate size
	if size > h.cfg.MaxUploadSize {
		_ = h.s3.DeleteRawObject(ctx, upload.RawObjectKey)
		_ = h.repo.UpdateStatusWithError(ctx, uploadID, domain.StatusFailed, "file too large")
		h.errorResponse(w, http.StatusBadRequest, "file too large")
		return
	}

	// Mark as uploaded and publish to queue
	if err := h.repo.UpdateStatus(ctx, uploadID, domain.StatusUploaded); err != nil {
		h.log.Error().Err(err).Msg("failed to update status")
		h.errorResponse(w, http.StatusInternalServerError, "failed to update status")
		return
	}

	// Publish processing message
	if err := h.publisher.PublishProcessImage(ctx, uploadID.String(), upload.RawObjectKey, string(upload.Purpose)); err != nil {
		h.log.Error().Err(err).Msg("failed to publish processing message")
		// Don't fail the request, worker will pick it up on retry
	}

	h.jsonResponse(w, http.StatusOK, CompleteUploadResponse{Status: string(domain.StatusProcessing)})
}

// GetStatus returns the current status of an upload.
func (h *UploadHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uploadIDStr := chi.URLParam(r, "id")

	uploadID, err := uuid.Parse(uploadIDStr)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid upload ID")
		return
	}

	upload, err := h.repo.GetByID(ctx, uploadID)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to get upload")
		h.errorResponse(w, http.StatusInternalServerError, "failed to get upload")
		return
	}
	if upload == nil {
		h.errorResponse(w, http.StatusNotFound, "upload not found")
		return
	}

	type StatusResponse struct {
		ID          string            `json:"id"`
		Status      string            `json:"status"`
		DerivedURLs map[string]string `json:"derived_urls,omitempty"`
		Error       string            `json:"error,omitempty"`
	}

	resp := StatusResponse{
		ID:     upload.ID.String(),
		Status: string(upload.Status),
		Error:  upload.ErrorMessage,
	}

	if upload.Status == domain.StatusReady && upload.DerivedKeys != nil {
		resp.DerivedURLs = make(map[string]string)
		for size, key := range upload.DerivedKeys {
			resp.DerivedURLs[size] = h.s3.PublicURL(key)
		}
	}

	h.jsonResponse(w, http.StatusOK, resp)
}

func (h *UploadHandler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *UploadHandler) errorResponse(w http.ResponseWriter, status int, message string) {
	h.jsonResponse(w, status, map[string]string{"error": message})
}
