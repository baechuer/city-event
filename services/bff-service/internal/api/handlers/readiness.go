package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ReadinessChecker checks if a dependency is ready.
type ReadinessChecker interface {
	Name() string
	Check(ctx context.Context) error
}

// HTTPReadinessChecker checks an HTTP endpoint for readiness.
type HTTPReadinessChecker struct {
	name string
	url  string
}

func NewHTTPReadinessChecker(name, url string) *HTTPReadinessChecker {
	return &HTTPReadinessChecker{name: name, url: url}
}

func (c *HTTPReadinessChecker) Name() string { return c.name }

func (c *HTTPReadinessChecker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &CheckError{Status: resp.StatusCode}
	}
	return nil
}

type CheckError struct {
	Status int
}

func (e *CheckError) Error() string {
	return "unhealthy status"
}

// ReadinessHandler handles /readyz and /healthz endpoints.
type ReadinessHandler struct {
	checkers []ReadinessChecker
}

func NewReadinessHandler(checkers ...ReadinessChecker) *ReadinessHandler {
	return &ReadinessHandler{checkers: checkers}
}

// Healthz is a simple liveness check (process is alive).
func (h *ReadinessHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Readyz checks all dependencies and returns detailed status.
func (h *ReadinessHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type checkResult struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	results := make([]checkResult, len(h.checkers))
	var wg sync.WaitGroup
	allHealthy := true

	for i, checker := range h.checkers {
		wg.Add(1)
		go func(idx int, c ReadinessChecker) {
			defer wg.Done()
			err := c.Check(ctx)
			if err != nil {
				results[idx] = checkResult{
					Name:   c.Name(),
					Status: "unhealthy",
					Error:  err.Error(),
				}
				allHealthy = false
			} else {
				results[idx] = checkResult{
					Name:   c.Name(),
					Status: "healthy",
				}
			}
		}(i, checker)
	}

	wg.Wait()

	resp := struct {
		Status string        `json:"status"`
		Checks []checkResult `json:"checks"`
	}{
		Status: "ready",
		Checks: results,
	}

	if !allHealthy {
		resp.Status = "not_ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
