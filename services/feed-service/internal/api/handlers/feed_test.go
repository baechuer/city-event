package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/infrastructure/postgres"
)

// Mock trending repo
type mockTrendingRepo struct {
	events []postgres.TrendingEvent
	err    error
}

func (m *mockTrendingRepo) GetTrending(ctx context.Context, city string, limit int, afterScore float64, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func (m *mockTrendingRepo) GetLatest(ctx context.Context, city string, limit int, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

// Mock profile repo
type mockProfileRepo struct {
	prefs map[string]float64
}

func (m *mockProfileRepo) GetUserProfile(ctx context.Context, actorKey string) (map[string]float64, error) {
	return m.prefs, nil
}

func TestFeedHandler_GetTrending(t *testing.T) {
	trendingRepo := &mockTrendingRepo{
		events: []postgres.TrendingEvent{
			{EventID: "1", Title: "Event 1", TrendScore: 10.0},
			{EventID: "2", Title: "Event 2", TrendScore: 8.0},
		},
	}
	profileRepo := &mockProfileRepo{}

	h := NewFeedHandler(trendingRepo, profileRepo)

	req := httptest.NewRequest("GET", "/api/feed?type=trending", nil)
	rr := httptest.NewRecorder()

	h.GetFeed(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["feed_type"] != "trending" {
		t.Errorf("expected feed_type 'trending', got '%v'", resp["feed_type"])
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestFeedHandler_DefaultFeedType(t *testing.T) {
	trendingRepo := &mockTrendingRepo{
		events: []postgres.TrendingEvent{
			{EventID: "1", Title: "Event 1", TrendScore: 10.0},
		},
	}
	profileRepo := &mockProfileRepo{}

	h := NewFeedHandler(trendingRepo, profileRepo)

	// No type specified, no user → defaults to trending
	req := httptest.NewRequest("GET", "/api/feed", nil)
	rr := httptest.NewRecorder()

	h.GetFeed(rr, req)

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["feed_type"] != "trending" {
		t.Errorf("expected 'trending', got '%v'", resp["feed_type"])
	}
}

func TestFeedHandler_PersonalizedWithUser(t *testing.T) {
	trendingRepo := &mockTrendingRepo{
		events: []postgres.TrendingEvent{
			{EventID: "1", Title: "Event 1", TrendScore: 10.0, Tags: []string{"music"}},
		},
	}
	profileRepo := &mockProfileRepo{
		prefs: map[string]float64{"music": 5.0},
	}

	h := NewFeedHandler(trendingRepo, profileRepo)

	// User header → defaults to personalized
	req := httptest.NewRequest("GET", "/api/feed", nil)
	req.Header.Set("X-User-ID", "user123")
	rr := httptest.NewRecorder()

	h.GetFeed(rr, req)

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["feed_type"] != "personalized" {
		t.Errorf("expected 'personalized', got '%v'", resp["feed_type"])
	}
}
