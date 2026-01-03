// PATH: services/bff-service/internal/downstream/httpclient.go
package downstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/logger"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
)

// ClientConfig holds configuration for the HTTP client wrapper
type ClientConfig struct {
	// ReadTimeout is used for GET requests
	ReadTimeout time.Duration
	// WriteTimeout is used for POST, PUT, PATCH, DELETE requests
	WriteTimeout time.Duration
}

// DefaultClientConfig returns sensible defaults
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
}

// Client is a centralized HTTP client wrapper that:
// 1. Injects X-Request-ID from context
// 2. Enforces timeouts based on HTTP method (read vs write)
// 3. Provides unified error mapping
// 4. Logs requests with correlation ID
type Client struct {
	baseClient *http.Client
	config     ClientConfig
}

// NewClient creates a new HTTP client wrapper
func NewClient(config ClientConfig) *Client {
	return &Client{
		baseClient: &http.Client{
			// No global timeout - we set per-request timeouts
			Timeout: 0,
		},
		config: config,
	}
}

// Do executes an HTTP request with:
// - Request-ID header injection
// - Method-based timeout enforcement
// - Unified error handling and logging
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 1. Inject X-Request-ID from context
	if reqID := middleware.GetRequestID(ctx); reqID != "" {
		req.Header.Set("X-Request-ID", reqID)
	}

	// 2. Determine timeout based on HTTP method
	timeout := c.config.ReadTimeout
	if isWriteMethod(req.Method) {
		timeout = c.config.WriteTimeout
	}

	// 3. Apply timeout to context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req = req.WithContext(ctx)

	// 4. Log the outgoing request
	log := logger.Log.With().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Str("request_id", middleware.GetRequestID(ctx)).
		Logger()

	start := time.Now()

	// 5. Execute request
	resp, err := c.baseClient.Do(req)

	// 6. Log result
	duration := time.Since(start)
	if err != nil {
		log.Warn().
			Err(err).
			Dur("duration", duration).
			Msg("downstream_request_failed")
		return nil, c.mapError(err)
	}

	log.Debug().
		Int("status", resp.StatusCode).
		Dur("duration", duration).
		Msg("downstream_request_completed")

	return resp, nil
}

// mapError converts low-level errors to domain errors
func (c *Client) mapError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrTimeout
	}
	// Connection refused, DNS errors, etc.
	return ErrUnavailable
}

// isWriteMethod returns true for HTTP methods that modify state
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// DoWithBody is a convenience method for requests with a body
func (c *Client) DoWithBody(ctx context.Context, method, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.Do(ctx, req)
}

// Get is a convenience method for GET requests
func (c *Client) Get(ctx context.Context, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.Do(ctx, req)
}
