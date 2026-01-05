package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/feed-service/internal/infrastructure/postgres"
	"github.com/baechuer/real-time-ressys/services/feed-service/internal/middleware"
)

// TrendingRepo defines the interface for trending data access
type TrendingRepo interface {
	GetTrending(ctx context.Context, city string, category string, queryStr string, limit int, afterScore float64, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error)
	GetLatest(ctx context.Context, city string, category string, queryStr string, limit int, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error)
}

// ProfileRepo defines the interface for user profile data access
type ProfileRepo interface {
	GetUserProfile(ctx context.Context, actorKey string) (map[string]float64, error)
}

// FeedHandler handles /feed requests
type FeedHandler struct {
	trendingRepo TrendingRepo
	profileRepo  ProfileRepo
	timeout      time.Duration
}

func NewFeedHandler(trendingRepo TrendingRepo, profileRepo ProfileRepo) *FeedHandler {
	return &FeedHandler{
		trendingRepo: trendingRepo,
		profileRepo:  profileRepo,
		timeout:      100 * time.Millisecond,
	}
}

// GetFeed handles GET /api/feed?type=trending|personalized|latest&city=&cursor=&limit=&q=&category=
func (h *FeedHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	feedType := r.URL.Query().Get("type")
	city := r.URL.Query().Get("city")
	category := r.URL.Query().Get("category")
	queryStr := r.URL.Query().Get("q")
	if queryStr == "" {
		queryStr = r.URL.Query().Get("search")
	}

	// Parse limit with default of 10
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	// Parse cursor
	var afterScore float64
	var afterStartTime time.Time
	var afterID string

	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		afterScore, afterStartTime, afterID = h.decodeCursor(cursor)
	}

	var events []postgres.TrendingEvent
	var err error

	// Default: logged-in → personalized, anonymous → trending
	if feedType == "" {
		if userID := r.Header.Get("X-User-ID"); userID != "" {
			feedType = "personalized"
		} else {
			feedType = "trending"
		}
	}

	switch feedType {
	case "personalized":
		events, err = h.getPersonalized(r, city, category, queryStr, limit, afterScore, afterStartTime, afterID)
	case "trending":
		events, err = h.getTrending(r.Context(), city, category, queryStr, limit, afterScore, afterStartTime, afterID)
	case "latest":
		events, err = h.getLatest(r.Context(), city, category, queryStr, limit, afterStartTime, afterID)
	default:
		http.Error(w, "invalid feed type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "failed to get feed", http.StatusInternalServerError)
		return
	}

	// Build pagination-compatible response
	var nextCursor string
	hasMore := false

	if len(events) > 0 {
		// If we got full page, assume there might be more
		// In a rigorous implementation we'd fetch limit+1 to know for sure
		if len(events) == limit {
			hasMore = true
			last := events[len(events)-1]
			nextCursor = h.encodeCursor(last.TrendScore, last.StartTime, last.EventID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"feed_type":   feedType,
		"items":       events,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// getTrending returns trending events with keyset pagination
func (h *FeedHandler) getTrending(ctx context.Context, city string, category string, queryStr string, limit int, afterScore float64, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	return h.trendingRepo.GetTrending(ctx, city, category, queryStr, limit, afterScore, afterStartTime, afterID)
}

// getLatest returns newest events ordered by start_time DESC
func (h *FeedHandler) getLatest(ctx context.Context, city string, category string, queryStr string, limit int, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	return h.trendingRepo.GetLatest(ctx, city, category, queryStr, limit, afterStartTime, afterID)
}

// getPersonalized returns personalized feed with fallback to trending
func (h *FeedHandler) getPersonalized(r *http.Request, city string, category string, queryStr string, limit int, afterScore float64, afterStartTime time.Time, afterID string) ([]postgres.TrendingEvent, error) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Get actor key
	var actorKey string
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		actorKey = "u:" + userID
	} else if anonID, ok := middleware.AnonIDFromContext(r.Context()); ok {
		actorKey = "a:" + anonID
	}

	// Get trending candidates (40ms budget)
	// CRITICAL FIX: Pass cursor parameters for proper pagination
	trendingCtx, trendingCancel := context.WithTimeout(ctx, 40*time.Millisecond)
	// Fetch more candidates than requested limit to allow for reranking diversity
	candidateLimit := limit * 10
	if candidateLimit > 200 {
		candidateLimit = 200
	}
	candidates, err := h.trendingRepo.GetTrending(trendingCtx, city, category, queryStr, candidateLimit, afterScore, afterStartTime, afterID)
	trendingCancel()
	if err != nil || len(candidates) == 0 {
		// Fallback to trending (simple pagination)
		return h.getTrending(r.Context(), city, category, queryStr, limit, afterScore, afterStartTime, afterID)
	}

	// Get user prefs (20ms budget)
	prefsCtx, prefsCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	prefs, _ := h.profileRepo.GetUserProfile(prefsCtx, actorKey)
	prefsCancel()

	// Rerank (10ms budget - in-memory operation)
	reranked := h.rerank(candidates, prefs)

	// Apply diversity (5ms - deterministic based on request_id)
	requestID := r.Header.Get("X-Request-ID")
	final := h.injectDiversity(reranked, requestID, 0.2)

	// Return top limit
	if len(final) > limit {
		final = final[:limit]
	}

	return final, nil
}

// encodeCursor creates a base64 string from pagination fields
func (h *FeedHandler) encodeCursor(score float64, startTime time.Time, id string) string {
	// Format: scoreBits(hex)|unixNano|id
	// Use Float64bits to ensure exact bitwise roundtrip of the float
	scoreBits := math.Float64bits(score)
	s := fmt.Sprintf("%x|%d|%s", scoreBits, startTime.UnixNano(), id)
	return base64.URLEncoding.EncodeToString([]byte(s))
}

// decodeCursor parses base64 string back to pagination fields
func (h *FeedHandler) decodeCursor(cursor string) (float64, time.Time, string) {
	// Try URLEncoding first (new standard)
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		// Fallback to StdEncoding (legacy/url-safe-std)
		b, err = base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			return 0, time.Time{}, ""
		}
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 3 {
		return 0, time.Time{}, ""
	}

	var score float64
	// Try parsing as hex (new format)
	scoreBits, err := strconv.ParseUint(parts[0], 16, 64)
	if err != nil {
		// Fallback to float parse (old format)
		if s, err2 := strconv.ParseFloat(parts[0], 64); err2 == nil {
			score = s
		}
	} else {
		score = math.Float64frombits(scoreBits)
	}

	ts, _ := strconv.ParseInt(parts[1], 10, 64)

	return score, time.Unix(0, ts), parts[2]
}

// rerank adjusts trending scores based on user preferences
func (h *FeedHandler) rerank(events []postgres.TrendingEvent, prefs map[string]float64) []postgres.TrendingEvent {
	if len(prefs) == 0 {
		return events
	}

	// Calculate tag affinity for each event
	for i := range events {
		tagAffinity := 0.0
		for _, tag := range events[i].Tags {
			if weight, ok := prefs[tag]; ok {
				tagAffinity += weight
			}
		}
		// Boost score by tag affinity (max 50% boost)
		boost := tagAffinity * 0.5
		if boost > events[i].TrendScore*0.5 {
			boost = events[i].TrendScore * 0.5
		}
		events[i].TrendScore += boost
	}

	// Sort by new score
	for i := 0; i < len(events)-1; i++ {
		for j := i + 1; j < len(events); j++ {
			if events[j].TrendScore > events[i].TrendScore {
				events[i], events[j] = events[j], events[i]
			}
		}
	}

	return events
}

// injectDiversity adds exploration items (deterministic based on requestID)
func (h *FeedHandler) injectDiversity(events []postgres.TrendingEvent, requestID string, ratio float64) []postgres.TrendingEvent {
	if len(events) < 10 || requestID == "" {
		return events
	}

	// Simple deterministic seed from requestID
	seed := int64(0)
	for _, c := range requestID {
		seed = seed*31 + int64(c)
	}

	// Move some items from tail to intersperse with head
	exploreCount := int(float64(len(events)) * ratio)
	if exploreCount < 2 {
		exploreCount = 2
	}

	// Every 5th item from the second half
	result := make([]postgres.TrendingEvent, 0, len(events))
	tailIdx := len(events) / 2
	tailInserted := 0

	for i, e := range events[:len(events)/2] {
		result = append(result, e)
		if (i+1)%5 == 0 && tailIdx < len(events) && tailInserted < exploreCount {
			result = append(result, events[tailIdx])
			tailIdx++
			tailInserted++
		}
	}
	// Append remaining
	result = append(result, events[len(events)/2:]...)

	return result
}
