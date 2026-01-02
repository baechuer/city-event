package downstream

import (
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
	ErrTimeout     = errors.New("downstream_timeout")
	ErrUnavailable = errors.New("downstream_unavailable")
	ErrNotFound    = errors.New("resource_not_found")
)

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
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var ev domain.Event
	if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
		return nil, err
	}

	return &ev, nil
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
	url := fmt.Sprintf("%s/join/v1/events/%s/participation", c.BaseURL, eventID)
	if userID != uuid.Nil {
		url = fmt.Sprintf("%s/join/v1/events/%s/participation/%s", c.BaseURL, eventID, userID)
	}

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
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var p struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}

	return &domain.Participation{
		EventID: eventID,
		UserID:  userID,
		Status:  domain.ParticipationStatus(p.Status),
	}, nil
}
func (c *JoinClient) JoinEvent(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) (domain.ParticipationStatus, error) {
	url := fmt.Sprintf("%s/join/v1/events/%s/join", c.BaseURL, eventID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var res struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return domain.ParticipationStatus(res.Status), nil
}

func (c *JoinClient) CancelJoin(ctx context.Context, eventID uuid.UUID, bearerToken, idempotencyKey, requestID string) error {
	url := fmt.Sprintf("%s/join/v1/events/%s/cancel", c.BaseURL, eventID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
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
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
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
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var raw struct {
		Items      []domain.EventCard `json:"items"`
		NextCursor string             `json:"next_cursor"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	return &domain.PaginatedResponse[domain.EventCard]{
		Items:      raw.Items,
		NextCursor: raw.NextCursor,
		HasMore:    raw.NextCursor != "",
	}, nil
}
