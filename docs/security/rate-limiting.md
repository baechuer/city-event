# Rate Limiting

## Purpose

Rate limiting reduces accidental or malicious request bursts against auth, mutation, and read endpoints. It is a security and resilience control, not a correctness guarantee.

## Implementation

The middleware is implemented in:

- `internal/platform/httpapi/rate_limit.go`
- `internal/platform/httpapi/router.go`
- `internal/platform/config/config.go`

It is attached to `httpapi.NewBaseRouter`, so it applies consistently to services using the shared router.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `RATE_LIMIT_ENABLED` | `true` | enable or disable middleware |
| `RATE_LIMIT_WINDOW` | `1m` | fixed-window duration |
| `RATE_LIMIT_REQUESTS` | `600` | read/general request limit per client/path/window |
| `RATE_LIMIT_AUTH_REQUESTS` | `60` | auth write limit per client/path/window |
| `RATE_LIMIT_MUTATION_REQUESTS` | `240` | non-auth write limit per client/path/window |

## Endpoint Scopes

The limiter classifies requests as:

- `auth`: `POST /v1/auth/...`
- `mutation`: other `POST`, `PATCH`, or `DELETE` requests
- `read`: all other non-exempt requests

Exempt paths:

- `/`
- `/livez`
- `/readyz`
- `/metrics`
- CORS preflight `OPTIONS`

When a request is rejected, the service returns HTTP `429` with:

- JSON error code `rate_limited`
- `Retry-After` header
- `cityevents_rate_limited_requests_total{service,scope}` metric increment

## Tests

Relevant tests:

- `internal/platform/config/config_test.go`
- `internal/platform/httpapi/router_test.go`

These tests verify configuration parsing, invalid config rejection, health/preflight exemptions, 429 behavior, `Retry-After`, and rate-limit metrics.

## Limitations

The current limiter is in-memory per process. In Kubernetes with two replicas, each pod has independent counters. That is acceptable for local hardening evidence, but it is not distributed rate limiting.

For production, replace or complement this with one of:

- Redis-backed shared counters
- NGINX ingress rate-limit annotations
- Envoy/API-gateway rate limiting
- managed WAF or edge gateway limits

The current client key uses the direct remote address. If CityEvents sits behind an ingress controller, production proxy trust rules must be explicit before using `X-Forwarded-For` for rate-limit identity.
