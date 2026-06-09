package feed

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
)

func TestFeedHandlersListAndDetail(t *testing.T) {
	start := time.Now().UTC().Add(24 * time.Hour)
	router := testFeedRouter(t, []Event{
		{EventID: "event-1", Title: "Tech", City: "Sydney", Venue: "Town Hall", StartsAt: &start, Status: StatusPublished, ConfirmedCount: 3},
		{EventID: "event-2", Title: "Art", City: "Melbourne", Venue: "Gallery", StartsAt: &start, Status: StatusPublished},
		{EventID: "event-3", Title: "Hidden", City: "Sydney", Venue: "Hall", StartsAt: &start, Status: "CANCELED"},
	})

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/feed/events?city=Sydney&limit=10&offset=0", nil)
	router.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Events []Event `json:"events"`
	}
	decodeFeedBody(t, list.Body.Bytes(), &listBody)
	if len(listBody.Events) != 1 || listBody.Events[0].EventID != "event-1" {
		t.Fatalf("list body = %+v", listBody)
	}

	detail := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/feed/events/event-1", nil)
	router.ServeHTTP(detail, req)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detail.Code, detail.Body.String())
	}
	if !bytes.Contains(detail.Body.Bytes(), []byte(`"confirmedCount":3`)) {
		t.Fatalf("detail missing confirmed count: %s", detail.Body.String())
	}
}

func TestFeedHandlersErrors(t *testing.T) {
	router := testFeedRouter(t, nil)

	badQuery := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/feed/events?limit=abc", nil)
	router.ServeHTTP(badQuery, req)
	if badQuery.Code != http.StatusBadRequest {
		t.Fatalf("bad query status = %d", badQuery.Code)
	}

	missing := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/feed/events/missing", nil)
	router.ServeHTTP(missing, req)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing detail status = %d", missing.Code)
	}
}

func testFeedRouter(t *testing.T, events []Event) http.Handler {
	t.Helper()
	cfg, err := config.Load("feed-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewHTTPHandler(cfg, nil, NewService(NewMemoryRepository(events), NewMemoryCache()))
}

func decodeFeedBody(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode body %s: %v", string(body), err)
	}
}
