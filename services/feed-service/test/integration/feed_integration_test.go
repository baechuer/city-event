//go:build integration
// +build integration

package integration

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeedEndpoint(t *testing.T) {
	baseURL := os.Getenv("FEED_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8084"
	}

	// Health check
	resp, err := http.Get(baseURL + "/api/feed/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	// Public Feed
	// /api/feed/
	resp, err = http.Get(baseURL + "/api/feed/?type=trending")
	if err != nil {
		t.Logf("feed check failed: %v", err)
	} else {
		defer resp.Body.Close()
		// We expect either 200 or 204 depending on if feed has items.
		// Assuming 200 for now.
		assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 204, "expected 200 or 204")
	}
}
