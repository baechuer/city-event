package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/baechuer/real-time-ressys/services/bff-service/internal/logger"
	"github.com/baechuer/real-time-ressys/services/bff-service/middleware"
)

// New creates a reverse proxy that rewrites paths and propagates context headers.
// targetHost: "http://auth-service:8080"
// stripPrefix: "/api/auth"
// upstreamPrefix: "/auth/v1"
func New(targetHost, stripPrefix, upstreamPrefix string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director

	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// 1. Host Header: Set to target's host so upstream feels like it's being called directly
		req.Host = target.Host

		// 2. Path Rewrite: /api/auth/login -> /auth/v1/login
		if strings.HasPrefix(req.URL.Path, stripPrefix) {
			req.URL.Path = upstreamPrefix + strings.TrimPrefix(req.URL.Path, stripPrefix)
		}

		// 3. Header Propagation
		reqID := middleware.GetRequestID(req.Context())
		if reqID != "" {
			req.Header.Set(middleware.HeaderXRequestID, reqID)
		}
	}

	// 4. Custom Error Handler (Upstream Down / Timeout)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		reqID := middleware.GetRequestID(r.Context())

		// Log the upstream failure
		logger.Log.Error().
			Err(err).
			Str("target", targetHost).
			Str("request_id", reqID).
			Msg("upstream_proxy_error")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)

		// Mimic Unified Error Format
		// We use a raw string to avoid struct dependencies
		w.Write([]byte(`{"error":{"code":"upstream_unavailable","message":"upstream service unreachable","request_id":"` + reqID + `"}}`))
	}

	return proxy, nil
}
