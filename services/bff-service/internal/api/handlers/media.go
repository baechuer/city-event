package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
	"github.com/google/uuid"
)

// MediaHandler handles media upload requests.
type MediaHandler struct {
	mediaServiceURL string
	httpClient      *http.Client
}

// NewMediaHandler creates a new media handler.
func NewMediaHandler(mediaServiceURL string) *MediaHandler {
	return &MediaHandler{
		mediaServiceURL: mediaServiceURL,
		httpClient:      &http.Client{},
	}
}

// RequestUpload proxies upload request to media-service.
func (h *MediaHandler) RequestUpload(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == uuid.Nil {
		h.errorResponse(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Create proxy request
	proxyURL := h.mediaServiceURL + "/media/v1/request-upload"
	req, err := http.NewRequestWithContext(r.Context(), "POST", proxyURL, r.Body)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", userID.String())
	req.Header.Set("X-Request-ID", r.Header.Get("X-Request-ID"))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.errorResponse(w, http.StatusBadGateway, "media service unavailable")
		return
	}
	defer resp.Body.Close()

	// Forward response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// CompleteUpload proxies complete request to media-service.
func (h *MediaHandler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == uuid.Nil {
		h.errorResponse(w, http.StatusUnauthorized, "authentication required")
		return
	}

	proxyURL := h.mediaServiceURL + "/media/v1/complete"
	req, err := http.NewRequestWithContext(r.Context(), "POST", proxyURL, r.Body)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", userID.String())
	req.Header.Set("X-Request-ID", r.Header.Get("X-Request-ID"))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.errorResponse(w, http.StatusBadGateway, "media service unavailable")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// GetStatus proxies status request to media-service.
func (h *MediaHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	// Status can be queried without auth (upload ID acts as token)
	proxyURL := h.mediaServiceURL + "/media/v1/status/" + r.PathValue("id")
	if proxyURL == "" {
		// Fallback for older Chi versions
		proxyURL = h.mediaServiceURL + r.URL.Path
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", proxyURL, nil)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	req.Header.Set("X-Request-ID", r.Header.Get("X-Request-ID"))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.errorResponse(w, http.StatusBadGateway, "media service unavailable")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *MediaHandler) errorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
