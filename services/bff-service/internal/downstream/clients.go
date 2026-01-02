package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrTimeout      = errors.New("downstream_timeout")
	ErrUnavailable  = errors.New("downstream_unavailable")
	ErrNotFound     = errors.New("resource_not_found")
	ErrUnauthorized = errors.New("unauthorized")
)

type StatusError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("downstream error [%d] %s: %s", e.StatusCode, e.Code, e.Message)
}

func decodeError(resp *http.Response) error {
	var apiErr domain.APIError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error.Code != "" {
		return &StatusError{
			StatusCode: resp.StatusCode,
			Code:       apiErr.Error.Code,
			Message:    apiErr.Error.Message,
		}
	}
	return &StatusError{
		StatusCode: resp.StatusCode,
		Code:       "downstream_error",
		Message:    fmt.Sprintf("unexpected status: %d", resp.StatusCode),
	}
}

type EventClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewEventClient(baseURL string) *EventClient {
	return &EventClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 2000 * time.Millisecond,
		},
	}
}

type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

func (c *EventClient) GetEvent(ctx context.Context, eventID uuid.UUID) (*domain.Event, error) {
	url := fmt.Sprintf("%s/event/v1/events/%s", c.BaseURL, eventID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var wrapper dataEnvelope[domain.Event]
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Data, nil
}

func (c *EventClient) CreateEvent(ctx context.Context, bearerToken string, body interface{}) (*domain.Event, error) {
	url := fmt.Sprintf("%s/event/v1/events", c.BaseURL)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", bearerToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var wrapper dataEnvelope[domain.Event]
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Data, nil
}

func (c *EventClient) PublishEvent(ctx context.Context, bearerToken, eventID string) (*domain.Event, error) {
	url := fmt.Sprintf("%s/event/v1/events/%s/publish", c.BaseURL, eventID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, err
	}

	if bearerToken != "" {
		req.Header.Set("Authorization", bearerToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var wrapper dataEnvelope[domain.Event]
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Data, nil
}

type JoinClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewJoinClient(baseURL string) *JoinClient {
	return &JoinClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 2000 * time.Millisecond,
		},
	}
}

func (c *JoinClient) GetParticipation(ctx context.Context, eventID, userID uuid.UUID, bearerToken string) (*domain.Participation, error) {
	url := fmt.Sprintf("%s/api/v1/events/%s/participation", c.BaseURL, eventID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if bearerToken != "" {
		req.Header.Set("Authorization", bearerToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
		return &domain.Participation{
			EventID: eventID,
			UserID:  userID,
			Status:  domain.StatusNone,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var wrapper dataEnvelope[struct {
		Status string `json:"status"`
	}]
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	return &domain.Participation{
		EventID: eventID,
		UserID:  userID,
		Status:  domain.ParticipationStatus(wrapper.Data.Status),
	}, nil
}
func (c *JoinClient) JoinEvent(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) (domain.ParticipationStatus, error) {
	url := fmt.Sprintf("%s/api/v1/join", c.BaseURL)
	body := map[string]string{"event_id": eventID.String()}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal join request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	if bearerToken != "" {
		req.Header.Set("Authorization", bearerToken)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		if body["code"] == "idempotency_key_mismatch" {
			return "", domain.ErrIdempotencyKeyMismatch
		}
		return "", domain.ErrAlreadyJoined
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", decodeError(resp)
	}

	var wrapper dataEnvelope[struct {
		Status string `json:"status"`
	}]
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return "", err
	}
	return domain.ParticipationStatus(wrapper.Data.Status), nil
}

func (c *JoinClient) CancelJoin(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) error {
	url := fmt.Sprintf("%s/api/v1/join/%s", c.BaseURL, eventID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	if bearerToken != "" {
		req.Header.Set("Authorization", bearerToken)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return decodeError(resp)
	}

	return nil
}

func (c *JoinClient) ListMyJoins(ctx context.Context, bearerToken string, query url.Values) (*domain.PaginatedResponse[domain.JoinRecord], error) {
	u, _ := url.Parse(fmt.Sprintf("%s/api/v1/me/joins", c.BaseURL))
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	if bearerToken != "" {
		req.Header.Set("Authorization", bearerToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	// NOTE: has_more 必须从上游读，不要用 next_cursor != "" 推断
	var wrapper dataEnvelope[struct {
		Items      []domain.JoinRecord `json:"items"`
		NextCursor string              `json:"next_cursor"`
		HasMore    bool                `json:"has_more"`
	}]

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	items := wrapper.Data.Items
	if items == nil {
		items = make([]domain.JoinRecord, 0)
	}

	return &domain.PaginatedResponse[domain.JoinRecord]{
		Items:      items,
		NextCursor: wrapper.Data.NextCursor,
		HasMore:    wrapper.Data.HasMore,
	}, nil
}

func (c *EventClient) ListEvents(ctx context.Context, query url.Values) (*domain.PaginatedResponse[domain.EventCard], error) {
	u, _ := url.Parse(fmt.Sprintf("%s/event/v1/events", c.BaseURL))
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	// NOTE: has_more 必须从上游读，不要用 next_cursor != "" 推断
	var wrapper dataEnvelope[struct {
		Items      []domain.EventCard `json:"items"`
		NextCursor string             `json:"next_cursor"`
		HasMore    bool               `json:"has_more"`
	}]

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	items := wrapper.Data.Items
	if items == nil {
		items = make([]domain.EventCard, 0)
	}

	return &domain.PaginatedResponse[domain.EventCard]{
		Items:      items,
		NextCursor: wrapper.Data.NextCursor,
		HasMore:    wrapper.Data.HasMore,
	}, nil
}

type AuthClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewAuthClient(baseURL string) *AuthClient {
	return &AuthClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 1000 * time.Millisecond,
		},
	}
}

func (c *AuthClient) GetUser(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	url := fmt.Sprintf("%s/internal/users/%s", c.BaseURL, userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var wrapper dataEnvelope[domain.User]
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Data, nil
}
