package eventregistration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/baechuer/cityevents/internal/platform/config"
	"github.com/baechuer/cityevents/internal/platform/identity"
)

func TestEventHandlersWorkflow(t *testing.T) {
	router, _ := testEventRouter(t)

	created := createEventViaHTTP(t, router, "organizer-1", 1)
	eventID := created.Event.ID

	joinOne := doEventJSON(router, http.MethodPost, "/v1/events/"+eventID+"/join", "", "user-1", nil)
	if joinOne.Code != http.StatusOK {
		t.Fatalf("join one status = %d body=%s", joinOne.Code, joinOne.Body.String())
	}
	var joinOneBody joinResponse
	decodeBody(t, joinOne.Body.Bytes(), &joinOneBody)
	if joinOneBody.Status != RegistrationStatusConfirmed {
		t.Fatalf("join one = %s, want confirmed", joinOneBody.Status)
	}

	duplicate := doEventJSON(router, http.MethodPost, "/v1/events/"+eventID+"/join", "", "user-1", nil)
	if duplicate.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d body=%s", duplicate.Code, duplicate.Body.String())
	}
	var duplicateBody joinResponse
	decodeBody(t, duplicate.Body.Bytes(), &duplicateBody)
	if !duplicateBody.Existing || duplicateBody.RegistrationID != joinOneBody.RegistrationID {
		t.Fatalf("duplicate did not return existing registration: %+v first=%+v", duplicateBody, joinOneBody)
	}

	joinTwo := doEventJSON(router, http.MethodPost, "/v1/events/"+eventID+"/join", "", "user-2", nil)
	if joinTwo.Code != http.StatusOK {
		t.Fatalf("join two status = %d body=%s", joinTwo.Code, joinTwo.Body.String())
	}
	var joinTwoBody joinResponse
	decodeBody(t, joinTwo.Body.Bytes(), &joinTwoBody)
	if joinTwoBody.Status != RegistrationStatusWaitlisted {
		t.Fatalf("join two = %s, want waitlisted", joinTwoBody.Status)
	}

	cancel := doEventJSON(router, http.MethodDelete, "/v1/events/"+eventID+"/join", "", "user-1", nil)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d body=%s", cancel.Code, cancel.Body.String())
	}
	var cancelBody cancelJoinResponse
	decodeBody(t, cancel.Body.Bytes(), &cancelBody)
	if cancelBody.Promoted == nil || cancelBody.Promoted.UserID != "user-2" {
		t.Fatalf("expected user-2 promotion, got %+v", cancelBody)
	}

	secondCancel := doEventJSON(router, http.MethodDelete, "/v1/events/"+eventID+"/join", "", "user-1", nil)
	if secondCancel.Code != http.StatusOK {
		t.Fatalf("second cancel status = %d body=%s", secondCancel.Code, secondCancel.Body.String())
	}

	status := doEventJSON(router, http.MethodGet, "/v1/events/"+eventID+"/join", "", "user-2", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", status.Code, status.Body.String())
	}
	if !bytes.Contains(status.Body.Bytes(), []byte(RegistrationStatusConfirmed)) {
		t.Fatalf("expected confirmed status body, got %s", status.Body.String())
	}
}

func TestEventHandlersValidationAndAuthorization(t *testing.T) {
	router, _ := testEventRouter(t)

	startsAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	body := `{"title":"Tech","description":"","city":"Sydney","venue":"Town Hall","startsAt":"` + startsAt + `","capacity":10}`
	unauthorized := doEventJSON(router, http.MethodPost, "/v1/events", body, "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("create without user status = %d", unauthorized.Code)
	}

	plainUser := doEventJSON(router, http.MethodPost, "/v1/events", body, "user-1", nil)
	if plainUser.Code != http.StatusForbidden {
		t.Fatalf("create as plain user status = %d", plainUser.Code)
	}

	invalid := doEventJSON(router, http.MethodPost, "/v1/events", `{"title":"","city":"Sydney","venue":"Town Hall","startsAt":"`+startsAt+`","capacity":10}`, "organizer-1", organizerHeaders())
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status = %d body=%s", invalid.Code, invalid.Body.String())
	}

	created := createEventViaHTTP(t, router, "organizer-1", 10)
	patch := `{"title":"Other"}`
	forbidden := doEventJSON(router, http.MethodPatch, "/v1/events/"+created.Event.ID, patch, "other-organizer", nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-organizer patch status = %d body=%s", forbidden.Code, forbidden.Body.String())
	}

	adminPatch := doEventJSON(router, http.MethodPatch, "/v1/events/"+created.Event.ID, patch, "admin-1", map[string]string{
		identity.HeaderUserRole: string(identity.RoleAdmin),
	})
	if adminPatch.Code != http.StatusOK {
		t.Fatalf("admin patch status = %d body=%s", adminPatch.Code, adminPatch.Body.String())
	}
}

func TestEventHandlersListExcludesCanceledAndDetailIncludesViewerStatus(t *testing.T) {
	router, _ := testEventRouter(t)
	active := createEventViaHTTP(t, router, "organizer-1", 10)
	canceled := createEventViaHTTP(t, router, "organizer-1", 10)
	_ = doEventJSON(router, http.MethodPost, "/v1/events/"+canceled.Event.ID+"/cancel", "", "organizer-1", organizerHeaders())
	_ = doEventJSON(router, http.MethodPost, "/v1/events/"+active.Event.ID+"/join", "", "viewer-1", nil)

	list := doEventJSON(router, http.MethodGet, "/v1/events", "", "", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	if bytes.Contains(list.Body.Bytes(), []byte(canceled.Event.ID)) {
		t.Fatalf("list included canceled event: %s", list.Body.String())
	}

	detail := doEventJSON(router, http.MethodGet, "/v1/events/"+active.Event.ID, "", "viewer-1", nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detail.Code, detail.Body.String())
	}
	if !bytes.Contains(detail.Body.Bytes(), []byte(RegistrationStatusConfirmed)) {
		t.Fatalf("detail did not include viewer join status: %s", detail.Body.String())
	}
}

func createEventViaHTTP(t *testing.T, router http.Handler, organizerID string, capacity int) eventDetailResponse {
	t.Helper()
	startsAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	body := `{"title":"Tech Meetup","description":"Monthly meetup","city":"Sydney","venue":"Town Hall","startsAt":"` + startsAt + `","capacity":` + strconv.Itoa(capacity) + `}`
	resp := doEventJSON(router, http.MethodPost, "/v1/events", body, organizerID, organizerHeaders())
	if resp.Code != http.StatusCreated {
		t.Fatalf("create event status = %d body=%s", resp.Code, resp.Body.String())
	}
	var detail eventDetailResponse
	decodeBody(t, resp.Body.Bytes(), &detail)
	return detail
}

func testEventRouter(t *testing.T) (http.Handler, *MemoryRepository) {
	t.Helper()
	repo := NewMemoryRepository()
	svc := NewService(repo)
	cfg, err := config.Load("event-registration-service", nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewHTTPHandler(cfg, nil, svc), repo
}

func doEventJSON(handler http.Handler, method, path, body, userID string, headers map[string]string) *httptest.ResponseRecorder {
	reqBody := bytes.NewReader([]byte(body))
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set(identity.HeaderUserID, userID)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func organizerHeaders() map[string]string {
	return map[string]string{identity.HeaderUserRole: string(identity.RoleOrganizer)}
}

func decodeBody(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode body %s: %v", string(body), err)
	}
}
