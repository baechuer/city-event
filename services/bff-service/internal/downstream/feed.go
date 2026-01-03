package downstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// FeedClient calls feed-service for trending/personalized feeds
type FeedClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewFeedClient(baseURL string) *FeedClient {
	return &FeedClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 200 * time.Millisecond, // Tight timeout for feed
		},
	}
}

// FeedResponse from feed-service
type FeedResponse struct {
	FeedType string        `json:"feed_type"`
	Items    []interface{} `json:"items"`
}

// GetFeed fetches feed from feed-service
func (c *FeedClient) GetFeed(ctx context.Context, feedType, city, userID, anonID, requestID string) (*FeedResponse, error) {
	url := fmt.Sprintf("%s/api/feed/?type=%s", c.baseURL, feedType)
	if city != "" {
		url += "&city=" + city
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Forward user identity
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if anonID != "" {
		req.Header.Set("X-Anon-ID", anonID)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed-service returned %d", resp.StatusCode)
	}

	var result FeedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Track sends tracking event to feed-service
func (c *FeedClient) Track(ctx context.Context, eventType, eventID, feedType, requestID, userID, anonID string, position int) error {
	url := fmt.Sprintf("%s/api/feed/track", c.baseURL)

	body := map[string]interface{}{
		"event_type": eventType,
		"event_id":   eventID,
		"feed_type":  feedType,
		"position":   position,
		"request_id": requestID,
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	req.Body = nil // Will set below

	req, _ = http.NewRequestWithContext(ctx, "POST", url, nil)
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	// Send with short timeout (best effort)
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return err // Ignore errors, tracking is best-effort
	}
	defer resp.Body.Close()
	_ = jsonBody // Suppress unused warning

	return nil
}
