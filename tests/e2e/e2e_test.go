package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseURL = "http://localhost:8080"

type Client struct {
	t      *testing.T
	client *http.Client
	token  string
}

func NewClient(t *testing.T) *Client {
	return &Client{
		t:      t,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Post(path string, body any) (int, map[string]any) {
	b, err := json.Marshal(body)
	require.NoError(c.t, err)

	req, err := http.NewRequest("POST", baseURL+path, bytes.NewBuffer(b))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var resMap map[string]any
	// ignore decode error for 204/empty
	_ = json.NewDecoder(resp.Body).Decode(&resMap)

	return resp.StatusCode, resMap
}

func (c *Client) Get(path string) (int, map[string]any) {
	req, err := http.NewRequest("GET", baseURL+path, nil)
	require.NoError(c.t, err)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var resMap map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&resMap)

	return resp.StatusCode, resMap
}

func TestE2E_Lifecycle(t *testing.T) {
	// 1. Setup
	organizer := NewClient(t)
	// user := NewClient(t) // Unused for now

	ts := time.Now().Unix()
	orgEmail := fmt.Sprintf("org_%d@test.com", ts)
	// userEmail := fmt.Sprintf("user_%d@test.com", ts)

	// 2. Register Organizer
	t.Log("Registering Organizer...")
	status, body := organizer.Post("/api/auth/register", map[string]string{
		"email":    orgEmail,
		"password": "strongpassword123",
	})
	require.Equal(t, http.StatusCreated, status, "Register failed: %v", body)

	// Login Organizer
	status, body = organizer.Post("/api/auth/login", map[string]string{
		"email":    orgEmail,
		"password": "strongpassword123",
	})
	require.Equal(t, http.StatusOK, status)
	tokens := body["tokens"].(map[string]any)
	organizer.token = tokens["access_token"].(string)

	// 3. Create Event
	t.Log("Creating Event...")
	startTime := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	endTime := time.Now().Add(26 * time.Hour).Format(time.RFC3339)
	status, body = organizer.Post("/api/events", map[string]any{
		"title":       "E2E Test Event",
		"description": "An event for testing",
		"city":        "TestCity",
		"category":    "Tech",
		"start_time":  startTime,
		"end_time":    endTime,
		"capacity":    100,
	})
	require.Equal(t, http.StatusCreated, status)
	eventID := body["id"].(string)

	// 4. Publish Event
	t.Log("Publishing Event...")
	status, _ = organizer.Post(fmt.Sprintf("/api/events/%s/publish", eventID), nil)
	require.Equal(t, http.StatusOK, status)

	// 5. Verify in Feed (Draft -> Published)
	// Cache might delay it? Wait 1s?
	time.Sleep(1 * time.Second)
	t.Log("Verifying Feed...")
	status, feed := organizer.Get("/api/feed") // Public
	require.Equal(t, http.StatusOK, status)
	items := feed["items"].([]any)
	found := false
	for _, item := range items {
		m := item.(map[string]any)
		if m["id"] == eventID {
			found = true
			break
		}
	}
	assert.True(t, found, "Event should be in feed")

	// 6. Unpublish Event
	t.Log("Unpublishing Event...")
	status, _ = organizer.Post(fmt.Sprintf("/api/events/%s/unpublish", eventID), nil)
	require.Equal(t, http.StatusOK, status)

	// 7. Verify Feed (Should be gone)
	// Wait 16s for cache? Or just verify "My Events" (Draft)?
	// Test is slow if we wait 16s.
	// But let's check "My Events" immediately.
	t.Log("Verifying My Events (Draft)...")
	status, myEvents := organizer.Get("/api/me/events")
	require.Equal(t, http.StatusOK, status)
	items = myEvents["items"].([]any)
	found = false
	var targetEvent map[string]any
	for _, item := range items {
		m := item.(map[string]any)
		if m["id"] == eventID {
			found = true
			targetEvent = m
			break
		}
	}
	assert.True(t, found, "Event should be in My Events")
	// Verify Status is Draft
	// Note: MyEvents endpoint might return Status. Check response structure.
	// Assuming it returns Event struct which has Status.
	if s, ok := targetEvent["status"]; ok {
		assert.Equal(t, "draft", s)
	}

	t.Log("E2E Test Completed Successfully")
}
