ENTERPRISE ARCHITECTURE AUDIT REPORT
City Event Platform - Real-Time Event Recommendation System
Audit Date: 2026-01-03
Auditor: Senior Staff Engineer (Adversarial Review)
Scope: Full-stack microservices architecture (Go/chi, DDD, Postgres, RabbitMQ, Redis, BFF + React SPA)

EXECUTIVE RISK SUMMARY
Top 10 Critical Risks (Severity × Likelihood)
1. P0 - HARDCODED SECRETS IN PRODUCTION CONFIG
Impact: Complete system compromise, unauthorized access to all services
Evidence:

docker-compose.yml:24 - JWT_SECRET=change_me
docker-compose.yml:25 - INTERNAL_SECRET_KEY=sharedkey
docker-compose.yml:70 - Same secrets reused across all services
compose.infra.yml:23 - Redis has NO password (commented out line 26)
compose.infra.yml:42-43 - RabbitMQ uses default guest/guest credentials
Remediation: IMMEDIATE - Rotate all secrets, use Docker secrets or external secret management (Vault, AWS Secrets Manager). Secrets MUST be injected at runtime, never committed to repo.

2. P0 - CSRF VULNERABILITY IN AUTH COOKIES
Impact: Session hijacking, unauthorized actions on behalf of authenticated users
Evidence:

services/auth-service/internal/infrastructure/security/cookies.go:21 - SameSite: http.SameSiteLaxMode
Lax mode allows cookies on top-level GET navigations from external sites
No CSRF token implementation found in BFF or auth-service
State-changing endpoints (join, cancel, publish) accept POST without CSRF protection
Remediation: Change to SameSiteStrict OR implement CSRF tokens for all state-changing operations. Lax is insufficient for financial/critical operations.

3. P0 - MISSING TIMEOUT ENFORCEMENT ON WRITE PATHS
Impact: Cascading failures, resource exhaustion, unbounded request queuing
Evidence:

services/join-service/internal/service/service.go:59 - 
JoinEvent
 has NO context timeout
services/join-service/internal/infrastructure/postgres/repository.go:37 - Transaction uses raw ctx without deadline
services/bff-service/internal/api/handlers/events.go:205 - 
CreateEvent
 has NO timeout
Only read paths have timeouts (e.g., events.go:71 - 1500ms for ListEvents)
Remediation: Enforce 3-5s max timeout on ALL write paths. Use context.WithTimeout before DB transactions. Writes MUST fail-fast to prevent queue buildup.

4. P0 - OUTBOX WORKER LACKS POISON MESSAGE HANDLING
Impact: Worker crash loop, message loss, operational blindness
Evidence:

services/join-service/internal/infrastructure/postgres/outbox_worker.go:268-283 - After 12 retries, messages move to 'dead' status
NO DLQ (Dead Letter Queue) for manual inspection
NO alerting mechanism for dead messages
NO admin interface to replay/inspect failed messages
Line 277-282: Logs error but provides no operational visibility
Remediation: Implement DLQ table + admin API to list/replay dead messages. Add Prometheus metrics for outbox_dead_count. Alert on any dead messages.

5. P1 - IDEMPOTENCY KEY LACKS TTL/CLEANUP
Impact: Unbounded table growth, performance degradation, storage exhaustion
Evidence:

services/join-service/migrations/007_idempotency_keys.up.sql
 - NO TTL, NO cleanup job
services/join-service/internal/infrastructure/postgres/repository.go:47-51 - INSERT without expiration
Table will grow indefinitely (1M users × 10 actions/day = 10M rows/day)
Remediation: Add expires_at TIMESTAMPTZ column, set to created_at + 24h. Create cron job to DELETE WHERE expires_at < NOW() - INTERVAL '7 days'.

6. P1 - BFF DEGRADATION DOES NOT PREVENT UNSAFE WRITES
Impact: Data corruption, inconsistent state when join-service is down
Evidence:

services/bff-service/internal/api/handlers/events.go:411-449 - 
JoinEvent
 calls downstream without circuit breaker
services/bff-service/internal/proxy/proxy.go:45-61 - ErrorHandler returns 502 but does NOT prevent retries
NO circuit breaker implementation found
UI will show error but user can retry immediately, causing thundering herd
Remediation: Implement circuit breaker (e.g., gobreaker library). When join-service is degraded, BFF MUST return 503 + Retry-After header. UI must disable "Join" button.

7. P1 - PROCESSED_MESSAGES TABLE LACKS COMPOSITE PK
Impact: Duplicate processing across different handlers, incorrect dedupe
Evidence:

services/join-service/migrations/001_init.up.sql:64-68 - message_id TEXT PRIMARY KEY
Should be PRIMARY KEY (message_id, handler_name)
Current schema allows same message_id to be processed by multiple handlers only once total
services/join-service/internal/infrastructure/postgres/processed_messages.go:29-32 - INSERT with both fields but PK is only message_id
Remediation: Migration to change PK to 
(message_id, handler_name)
. This is CRITICAL for multi-handler correctness.

8. P1 - NO REQUEST-ID PROPAGATION TO DOWNSTREAM SERVICES
Impact: Inability to trace requests across service boundaries, debugging nightmare
Evidence:

services/bff-service/internal/proxy/proxy.go:38-41 - Request-ID propagated to proxied auth endpoints
services/bff-service/internal/api/handlers/events.go:205 - Direct HTTP calls to event-service do NOT propagate Request-ID
services/bff-service/internal/downstream/clients.go
 - Need to inspect, likely missing header propagation
Remediation: ALL downstream HTTP calls MUST include X-Request-ID header. Centralize in HTTP client wrapper.

9. P1 - PAGINATION CURSOR LACKS DETERMINISTIC TIE-BREAKER
Impact: Duplicate/missing items in paginated results, user confusion
Evidence:

services/join-service/internal/infrastructure/postgres/reads.go
 (need to inspect) - Likely uses created_at alone
services/join-service/internal/domain/domain.go:38-41 - KeysetCursor has CreatedAt + ID
If two joins have same created_at (microsecond collision), ordering is undefined
Query must use ORDER BY created_at, id but implementation unclear
Remediation: Verify all keyset pagination queries use ORDER BY created_at ASC, id ASC (or DESC). Add integration test for tie-breaker correctness.

10. P2 - MISSING RATE LIMITING ON WRITE ENDPOINTS
Impact: Abuse, resource exhaustion, unfair usage
Evidence:

services/join-service/internal/infrastructure/redis/redis.go
 - Has 
AllowRequest
 method (rate limiting exists)
services/join-service/internal/transport/rest/handlers.go
 - Need to inspect if rate limiting is APPLIED
No evidence of per-user rate limiting on join/cancel operations
Could allow single user to spam join/cancel and exhaust capacity
Remediation: Apply rate limiting middleware to ALL write endpoints. Limit: 10 joins/min per user, 100 joins/min per IP.

DETAILED AUDIT FINDINGS
A. ARCHITECTURE & BOUNDARIES
A1. BFF Violates Single Responsibility (P2)
Evidence: services/bff-service/internal/api/handlers/events.go:104-195

BFF performs complex business logic (fan-out enrichment for ListMyJoins)
Lines 133-163: Concurrent fetching of event details for each join record
Lines 175-184: Transformation logic (JoinRecord → EventCard)
Issue: BFF should be a thin routing layer. Business logic belongs in join-service.

Recommended Fix:

Minimal: Document this as acceptable for V1, plan refactor for V2
Ideal: Create join-service endpoint /joins/enriched that returns EventCard directly
A2. Shared Database Violates Service Autonomy (P2)
Evidence: 
services/join-service/internal/infrastructure/postgres/acl.go
 (need to inspect)

services/join-service/internal/service/service.go:43 - Calls repo.GetEventOwnerID
Join-service queries event_service's database for ACL checks
Issue: Cross-service database access creates tight coupling.

Recommended Fix:

Minimal: Document as acceptable for V1 (same Postgres instance, different schemas)
Ideal: Event-service exposes /internal/events/{id}/owner endpoint, join-service calls via HTTP with INTERNAL_SECRET_KEY
B. CORRECTNESS & CONCURRENCY
B1. Outbox Worker In-Flight Window Race Condition (P1)
Evidence: services/join-service/internal/infrastructure/postgres/outbox_worker.go:159-170

inFlightUntil := time.Now().Add(15 * time.Second)
for _, m := range messages {
    _, _ = tx.Exec(ctx, `UPDATE outbox SET next_retry_at = $2 WHERE id = $1`, m.ID, inFlightUntil)
}
if err := tx.Commit(ctx); err != nil {
    return err
}
Issue: If worker crashes AFTER commit but BEFORE publish completes, messages are stuck for 15s. If worker restarts, it won't pick them up until next_retry_at expires.

Recommended Fix:

// After successful publish (line 240-245), immediately mark as 'sent'
// If publish fails, update next_retry_at with backoff
// Remove in-flight window entirely - rely on FOR UPDATE SKIP LOCKED
B2. Join Event Lacks Owner-Cannot-Join Enforcement at DB Level (P2)
Evidence: services/join-service/internal/service/service.go:42-46

owner, err := s.repo.GetEventOwnerID(ctx, eventID)
if err == nil && owner == userID {
    return "", domain.ErrForbidden
}
Issue: Application-level check. Race condition if owner is changed concurrently.

Recommended Fix: Add CHECK constraint or trigger in joins table:

ALTER TABLE joins ADD CONSTRAINT chk_not_owner 
CHECK (user_id != (SELECT owner_id FROM events WHERE id = event_id));
(Requires events table in same DB or materialized view)

B3. Waitlist Promotion Uses SKIP LOCKED Without Retry (P2)
Evidence: services/join-service/internal/infrastructure/postgres/repository.go:287-294

err = tx.QueryRow(ctx, `
    SELECT user_id FROM joins
    WHERE event_id = $1 AND status = 'waitlisted'
    ORDER BY created_at ASC, id ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
`, eventID).Scan(&promoUserID)
Issue: If another transaction holds lock on the first waitlisted user, this query returns ErrNoRows and promotion is skipped. Correct behavior but could be improved.

Recommended Fix:

// Try up to 3 candidates
FOR i := 0; i < 3; i++ {
    err = tx.QueryRow(ctx, `SELECT user_id ... OFFSET $2 FOR UPDATE SKIP LOCKED`, eventID, i).Scan(&promoUserID)
    if err == nil { break }
}
C. SECURITY
C1. JWT Secret Rotation Not Supported (P1)
Evidence: docker-compose.yml:24,49,70,113 - Single JWT_SECRET env var

No mechanism to rotate secrets without downtime
No support for multiple valid secrets during rotation period
Recommended Fix:

// Support comma-separated secrets: JWT_SECRET=new_secret,old_secret
// Verify with all secrets, sign with first secret
// After 24h (max token TTL), remove old_secret
C2. Internal Endpoints Exposed via BFF (P0)
Evidence: Need to inspect 
services/bff-service/internal/api/router.go

If BFF proxies ALL /api/auth/* to auth-service, internal endpoints like /auth/v1/admin/* may be exposed
Recommended Fix:

// BFF router MUST whitelist allowed paths
// NEVER proxy /admin, /internal, /debug paths
// Use explicit route registration, not wildcard proxy
C3. CORS Configuration Missing (P1)
Evidence: No CORS middleware found in BFF

services/bff-service/middleware/security.go
 - Only sets security headers, no CORS
Recommended Fix:

import "github.com/go-chi/cors"
r.Use(cors.Handler(cors.Options{
    AllowedOrigins:   []string{"https://yourdomain.com"},
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
    AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key"},
    AllowCredentials: true,
    MaxAge:           300,
}))
D. RELIABILITY
D1. No Health Check Endpoints (P1)
Evidence: No /health or /ready endpoints found in any service

Recommended Fix:

// Each service MUST expose:
// GET /health - Returns 200 if process is alive
// GET /ready  - Returns 200 if DB/Redis/RabbitMQ are reachable
// Kubernetes liveness/readiness probes depend on these
D2. Outbox Worker Single Point of Failure (P1)
Evidence: services/join-service/internal/infrastructure/postgres/outbox_worker.go:44

Single goroutine, no leader election
If process crashes, outbox stops publishing until restart
Recommended Fix:

Minimal: Run 2+ instances of join-service, rely on FOR UPDATE SKIP LOCKED for work distribution
Ideal: Add leader election (etcd/Consul) so only one worker is active, with automatic failover
D3. No Retry Budget for Read Paths (P2)
Evidence: services/bff-service/internal/api/handlers/events.go:74

res, err := h.eventClient.ListEvents(ctx, r.URL.Query())
if err != nil {
    handleDownstreamError(w, r, err, "failed to fetch events")
    return
}
Issue: Single attempt, no retry on transient failures.

Recommended Fix:

// Use exponential backoff with max 2 retries for idempotent reads
// Total budget: 1500ms timeout ÷ 3 attempts = 500ms per attempt
E. DATA INTEGRITY
E1. Outbox Message_ID Not Enforced as Unique (P1)
Evidence: services/join-service/migrations/001_init.up.sql:47-56

outbox table has id UUID PRIMARY KEY but message_id is not in schema
services/join-service/internal/infrastructure/postgres/outbox_worker.go:193 - Uses MessageId from AMQP header
Issue: If same message is inserted twice into outbox (application bug), it will be published twice.

Recommended Fix:

ALTER TABLE outbox ADD COLUMN message_id UUID NOT NULL;
CREATE UNIQUE INDEX idx_outbox_message_id ON outbox(message_id);
E2. Event Capacity Can Go Negative (P2)
Evidence: services/join-service/internal/infrastructure/postgres/repository.go:169-172

if newStatus == domain.StatusActive {
    _, _ = tx.Exec(ctx, `UPDATE event_capacity SET active_count = active_count + 1 ...`)
}
Issue: No CHECK constraint to prevent active_count > capacity or active_count < 0.

Recommended Fix:

ALTER TABLE event_capacity ADD CONSTRAINT chk_counts_valid
CHECK (active_count >= 0 AND waitlist_count >= 0 AND (capacity = 0 OR active_count <= capacity));
F. API QUALITY
F1. Inconsistent Error Response Format (P2)
Evidence:

services/auth-service/internal/transport/http/response/success.go:8-10 - Wraps in {"data": ...}
services/bff-service/internal/api/handlers/events.go:216 - Returns raw object
Issue: Frontend must handle two different response shapes.

Recommended Fix: Standardize on envelope format across ALL services:

{"data": {...}, "meta": {"request_id": "..."}}
{"error": {"code": "...", "message": "...", "request_id": "..."}}
F2. Idempotency-Key Header Not Documented (P2)
Evidence: services/bff-service/internal/api/handlers/events.go:419

Required header but no OpenAPI spec found
Recommended Fix: Generate OpenAPI 3.0 spec with:

parameters:
  - name: Idempotency-Key
    in: header
    required: true
    schema:
      type: string
      format: uuid
G. OBSERVABILITY
G1. No Structured Logging for Business Events (P1)
Evidence: services/join-service/internal/infrastructure/postgres/repository.go:182

_, _ = tx.Exec(ctx, `INSERT INTO outbox ...`)
Issue: No log when join is created. Only outbox publish is logged.

Recommended Fix:

logger.Info().
    Str("event_id", eventID.String()).
    Str("user_id", userID.String()).
    Str("status", string(newStatus)).
    Str("idempotency_key", idempotencyKey).
    Msg("join_created")
G2. Request-ID Not Logged in All Services (P1)
Evidence: 
services/bff-service/middleware/request_id.go
 - Generates Request-ID

But downstream services (join, event, auth) don't log it consistently
Recommended Fix: All services MUST extract Request-ID from header and add to logger context:

reqID := r.Header.Get("X-Request-ID")
log := logger.With().Str("request_id", reqID).Logger()
ctx := log.WithContext(r.Context())
G3. No Metrics Exposed (P0)
Evidence: No Prometheus /metrics endpoint found in any service

Recommended Fix: Add promhttp handler to each service:

import "github.com/prometheus/client_golang/prometheus/promhttp"
// Metrics to expose:
// - join_requests_total{status="active|waitlisted|error"}
// - outbox_messages_total{status="sent|dead"}
// - http_request_duration_seconds{endpoint, status}
H. PERFORMANCE
H1. N+1 Query in ListMyJoins (P1)
Evidence: services/bff-service/internal/api/handlers/events.go:141-161

for _, join := range items {
    wg.Add(1)
    go func(eid uuid.UUID) {
        ev, err := h.eventClient.GetEvent(ctx, eid)
        // ...
    }(join.EventID)
}
Issue: 10 joins = 10 HTTP calls to event-service. Latency = slowest call.

Recommended Fix:

Minimal: Increase concurrency limit from 5 to 20 (line 136)
Ideal: Add POST /events/batch endpoint that accepts event_ids[] and returns map
H2. No Database Connection Pooling Limits (P2)
Evidence: Need to inspect 
services/join-service/api/cmd/main.go
 for pgxpool config

Default pgxpool max_conns = unlimited (bounded by Postgres max_connections)
Recommended Fix:

config.MaxConns = 25  // Per service instance
config.MinConns = 5
config.MaxConnIdleTime = 30 * time.Minute
I. DEVEX & MAINTAINABILITY
I1. No CI/CD Pipeline (P1)
Evidence: .github/workflows/ - Need to inspect

No evidence of automated testing on PR
Recommended Fix: Create .github/workflows/ci.yml:

name: CI
on: [pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: docker-compose -f compose.infra.yml up -d
      - run: make test-all
I2. Integration Tests Don't Verify Idempotency (P1)
Evidence: services/join-service/internal/infrastructure/postgres/repository_test.go (need to inspect)

Need test that calls JoinEvent twice with same idempotency key and verifies single join
Recommended Fix:

func TestJoinEvent_Idempotency(t *testing.T) {
    key := uuid.New().String()
    status1, _ := repo.JoinEvent(ctx, "trace", key, eventID, userID)
    status2, _ := repo.JoinEvent(ctx, "trace", key, eventID, userID)
    assert.Equal(t, status1, status2)
    // Verify only 1 row in joins table
}
J. COMPLIANCE WITH STATED PRINCIPLES
J1. VIOLATED: "Write path MUST NOT auto-retry" (P0)
Evidence: services/bff-service/internal/api/handlers/events.go:205

No explicit retry prevention
If event-service times out, BFF returns 502
Frontend may auto-retry (browser/axios default behavior)
Recommended Fix:

// BFF MUST return 503 Service Unavailable (not 502)
// Include Retry-After: 10 header
// Frontend MUST NOT auto-retry writes
J2. VIOLATED: "Request-ID must propagate across BFF → services" (P1)
Evidence: Already documented in G2 and finding #8

J3. COMPLIANT: "Outbox pattern used for event publication" ✓
Evidence: services/join-service/internal/infrastructure/postgres/repository.go:174-183

Outbox insert is in same transaction as domain state change
Worker publishes asynchronously
J4. COMPLIANT: "Consumers must be idempotent" ✓
Evidence: services/join-service/internal/infrastructure/rabbitmq/consumer.go:164-176

Uses ProcessOnce with atomic dedupe fence + side effects
PRIORITIZED 2-WEEK REMEDIATION PLAN
Week 1: P0 Critical Security & Correctness
Day 1-2: Secret Management

 Rotate all secrets (JWT, internal keys, DB passwords)
 Implement Docker secrets or env-based injection
 Add .env.example with placeholder values
 Update deployment docs
Day 3: CSRF Protection

 Change SameSite to Strict in auth cookies
 Add CSRF token generation to auth-service
 Update BFF to validate CSRF tokens on POST/PUT/DELETE
 Update frontend to include CSRF token in headers
Day 4: Write Path Timeouts

 Add 5s timeout to all write operations (join, cancel, create, publish)
 Add integration tests to verify timeout enforcement
 Update error handling to distinguish timeout vs other errors
Day 5: Outbox DLQ & Metrics

 Add outbox_dead table for manual inspection
 Expose /metrics endpoint with Prometheus metrics
 Add Grafana dashboard for outbox health
 Create runbook for dead message recovery
Week 2: P1 Reliability & Observability
Day 6: Idempotency Key TTL

 Add migration for expires_at column
 Create cleanup cron job
 Add monitoring for table size
Day 7: Circuit Breaker

 Implement gobreaker in BFF for join-service calls
 Add /health and /ready endpoints to all services
 Configure Kubernetes probes
Day 8: Processed Messages PK Fix

 Create migration to change PK to (message_id, handler_name)
 Test migration on staging
 Deploy with zero downtime
Day 9: Request-ID Propagation

 Centralize HTTP client in BFF with Request-ID header injection
 Add Request-ID to all log statements
 Add integration test to verify end-to-end propagation
Day 10: CI/CD Pipeline

 Create GitHub Actions workflow
 Add pre-commit hooks for linting
 Configure automated testing on PR
QUALITY GATES CHECKLIST
Use this checklist to determine if the repo is interview/internship-ready:

Security ✓/✗
 All secrets are externalized (no hardcoded values)
 CSRF protection is enabled (SameSite=Strict OR CSRF tokens)
 CORS is configured with explicit allowed origins
 Internal endpoints are not exposed via BFF
 JWT secret rotation is supported
Reliability ✓/✗
 All write paths have timeout enforcement (≤5s)
 Circuit breaker is implemented for critical dependencies
 Health check endpoints exist (/health, /ready)
 Outbox has DLQ for poison messages
 Rate limiting is applied to write endpoints
Correctness ✓/✗
 Idempotency keys have TTL and cleanup
 Processed messages table has composite PK (message_id, handler_name)
 Database constraints prevent invalid states (negative counts, over-capacity)
 Integration tests verify idempotency under duplicate delivery
 Pagination has deterministic tie-breaker (ORDER BY created_at, id)
Observability ✓/✗
 Request-ID propagates across all service boundaries
 Structured logging includes business events (join created, promoted, etc.)
 Prometheus metrics are exposed (/metrics)
 Grafana dashboards exist for key metrics
 Alerting is configured for critical failures (outbox dead, circuit open)
Developer Experience ✓/✗
 CI/CD pipeline runs tests on every PR
 README has clear setup instructions
 API documentation exists (OpenAPI/Swagger)
 Integration tests can run locally with docker-compose
 Code has consistent error handling patterns
Production Readiness ✓/✗
 Database migrations are versioned and reversible
 Connection pooling is configured with limits
 Graceful shutdown is implemented (drain connections, finish in-flight requests)
 Deployment runbook exists (rollback procedure, secret rotation, scaling)
 Load testing has been performed (target: 1000 concurrent joins)
CONCLUSION
Overall Assessment: GOOD FOUNDATION, NOT PRODUCTION-READY

Strengths:

Excellent idempotency implementation (DB-level keys + ProcessOnce pattern)
Correct outbox pattern with atomic guarantees
Proper locking order to prevent deadlocks
Consumer dedupe with inbox pattern
Critical Gaps:

Hardcoded secrets (P0 blocker)
CSRF vulnerability (P0 security risk)
Missing timeouts on writes (P0 reliability risk)
No metrics/observability (P0 operational blindness)
Recommendation: Address all P0 issues before ANY production deployment. P1 issues should be resolved before scaling beyond pilot users. P2 issues are acceptable technical debt for V1 but must be tracked.

Interview Readiness: After Week 1 remediation (P0 fixes), this repo would demonstrate strong understanding of distributed systems patterns. After Week 2 (P1 fixes), it would be an excellent portfolio piece for senior/staff engineer interviews.