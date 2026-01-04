# Production Readiness Audit Report

**Project**: CityEvents Platform  
**Audit Date**: 2026-01-04  
**Verdict**: ⚠️ **CONDITIONAL GO** (Staging/Demo Ready; Production requires security hardening)

---

## Executive Summary

The platform is **functionally complete** with all P0 logic bugs resolved. This audit organizes findings by priority: Security → Performance → Scalability → Observability → Operational.

---

## 1. 🔐 Security Assessment

### 1.1 Authentication & Authorization

| Component | Status | Finding |
|-----------|--------|---------|
| **JWT Signing** | ⚠️ Needs Work | Single `JWT_SECRET` with no `kid` rotation support |
| **Token Storage** | ✅ Good | B1 pattern: HttpOnly cookie (refresh) + in-memory (access) |
| **CSRF Protection** | ✅ Good | Origin/Referer validation on mutation endpoints |
| **Password Hashing** | ✅ Good | bcrypt with appropriate cost factor |
| **OAuth State Validation** | ⚠️ Unverified | State parameter used but validation not audited |

**Key Gaps**:
- 🔴 **No key rotation**: If `JWT_SECRET` leaks, must restart all services and invalidate all sessions
- 🟡 **BFF opportunistic auth**: `middleware.Auth` extracts token but doesn't enforce authentication

**Recommendations**:
1. Implement JWKS-style `kid` header for key rotation
2. Add `middleware.RequireAuth` to BFF protected routes

### 1.2 Security Headers

| Header | Status | Current Value |
|--------|--------|---------------|
| Content-Security-Policy | ✅ Set | `default-src 'self'; script-src 'self'` |
| X-Content-Type-Options | ✅ Set | `nosniff` |
| X-Frame-Options | ✅ Set | `DENY` |
| Strict-Transport-Security | ❌ Missing | Not set |
| Referrer-Policy | ✅ Set | `strict-origin-when-cross-origin` |

**Recommendations**:
1. Add HSTS header: `Strict-Transport-Security: max-age=31536000; includeSubDomains`
2. Review CSP for production (may need adjustments for CDN assets)

### 1.3 Input Validation & Rate Limiting

| Endpoint | Rate Limit | Status |
|----------|------------|--------|
| `/auth/login` | 10 req/min | ✅ Strict |
| `/auth/register` | 5 req/min | ✅ Strict |
| General API | 100 req/min | ✅ Configured |
| `/join` mutations | Per-user | ✅ + Idempotency |

**Idempotency**: ✅ Fixed - Handler validates `X-Idempotency-Key` presence before processing.

---

## 2. ⚡ Performance Assessment

### 2.1 Database Performance

| Query Type | Optimization | Status |
|------------|--------------|--------|
| Event List | Keyset pagination | ✅ O(1) regardless of page depth |
| Feed Trending | Indexed by `trend_score` | ✅ Good |
| Join Lookup | `FOR UPDATE` locks | ⚠️ Lock contention possible at high load |
| User Lookup | Indexed by email | ✅ Good |

**Known Bottlenecks**:
- 🟡 `rerank()` uses O(N²) bubble sort for 200 items → Use `sort.Slice`
- 🟡 No connection pooling limits specified → Add `max_connections` config

### 2.2 Caching Strategy

| Layer | Cache | TTL | Status |
|-------|-------|-----|--------|
| Event Detail | Redis | 5 min | ✅ Enabled |
| Event List | Redis | 1 min | ✅ Enabled |
| User Session | Redis | 7 days | ✅ Enabled |
| Feed Results | None | - | 🟡 Future optimization |

### 2.3 Database Indexes

| Table | Column | Index Status |
|-------|--------|--------------|
| `events` | `id` | ✅ Primary |
| `events` | `status, start_time` | ✅ Composite |
| `events` | `owner_id` | ✅ Indexed |
| `users` | `email` | ✅ Unique |
| `users` | `role` | ❌ Missing (used in admin queries) |
| `joins` | `event_id, user_id` | ✅ Composite |

---

## 3. 📈 Scalability & Capacity

### 3.1 Estimated Throughput (Single Instance)

| Service | Estimated QPS | Limiting Factor |
|---------|---------------|-----------------|
| **BFF** | ~500 QPS | Go HTTP server, stateless |
| **Event Service** (reads) | ~300 QPS | DB connection pool |
| **Event Service** (writes) | ~50 WPS | Outbox worker + DB writes |
| **Join Service** | ~100 WPS | `FOR UPDATE` lock serialization |
| **Feed Service** | ~200 QPS | In-memory reranking |

*These are estimates based on typical Go service performance. Actual numbers require load testing.*

### 3.2 Scaling Strategy

| Service | Horizontal Scaling | Notes |
|---------|-------------------|-------|
| **BFF** | ✅ Stateless | Scale freely |
| **Event Service** | ✅ Stateless | Scale freely, DB is bottleneck |
| **Join Service** | ⚠️ Limited | Lock contention increases with replicas |
| **Feed Service** | ✅ Stateless | Scale freely |
| **Email Service** | ✅ Consumer | Increase prefetch for throughput |

**Database Scaling**:
- Current: Single Postgres per service
- Growth path: Read replicas → Connection pooling (PgBouncer) → Sharding

### 3.3 Message Queue Capacity

| Setting | Current Value | Recommendation |
|---------|---------------|----------------|
| Consumer Prefetch | 10 | Increase to 50-100 for throughput |
| Publisher Confirms | Enabled | ✅ Good |
| DLQ | ❌ Not configured | Add `x-dead-letter-exchange` |

---

## 4. 📊 Observability

### 4.1 Logging

| Aspect | Status | Details |
|--------|--------|---------|
| Structured Logging | ✅ Yes | JSON format via `zerolog` |
| Request ID Propagation | ✅ Yes | `X-Request-ID` across all services |
| Log Level Config | ✅ Yes | Configurable via env |
| Sensitive Data Masking | ⚠️ Partial | Passwords not logged, but some user IDs visible |

**Log Fields Present**:
```json
{
  "level": "info",
  "method": "POST",
  "path": "/join/v1/join",
  "status": 200,
  "latency": 45,
  "request_id": "abc-123",
  "user_id": "uuid"
}
```

### 4.2 Metrics

| Metric Type | Status | Tool |
|-------------|--------|------|
| HTTP Request Count | ✅ Enabled | Prometheus middleware |
| Request Latency Histogram | ✅ Enabled | Prometheus middleware |
| RabbitMQ Queue Depth | ❌ Not exposed | Need RabbitMQ exporter |
| DB Connection Pool | ❌ Not exposed | Add `sql.DBStats` exporter |

### 4.3 Distributed Tracing

| Aspect | Status | Notes |
|--------|--------|-------|
| Trace Propagation | ✅ Enabled | Via `X-Request-ID` |
| Jaeger Integration | ✅ Configured | In `docker-compose` |
| Span Creation | ⚠️ Basic | Only at HTTP layer |
| Trace Sampling | Not configured | Default 100% |

---

## 5. 🔧 Operational Readiness

### 5.1 Health Checks

| Endpoint | Checks | Status |
|----------|--------|--------|
| `/healthz` | HTTP Server up | ✅ All services |
| `/readyz` | DB + Redis connectivity | ✅ Fixed (DB ping added) |

### 5.2 Graceful Shutdown

| Service | Graceful Shutdown | Status |
|---------|-------------------|--------|
| BFF | ⚠️ Basic | `http.Server.Shutdown` |
| Event | ⚠️ Basic | No drain period for outbox |
| Join | ⚠️ Basic | Consumer stops immediately |

**Recommendation**: Add drain period for in-flight requests and outbox worker.

### 5.3 Configuration Management

| Aspect | Status |
|--------|--------|
| Environment Variables | ✅ Used consistently |
| Secrets Management | ⚠️ Plain env vars (use K8s Secrets/Vault) |
| Config Validation | ✅ Fails fast on missing required config |

---

## 6. Deployment Readiness Checklist

### Pre-Deploy (Must Do)
- [ ] Set strong `JWT_SECRET` (not dev default)
- [ ] Run database migrations
- [ ] Verify RabbitMQ exchanges/queues created
- [ ] Configure DNS/hosts for domain

### Recommended Before Production
- [ ] Add HSTS header
- [ ] Configure DLQ for RabbitMQ
- [ ] Add index on `users.role`
- [ ] Load test to establish baseline QPS

### Nice to Have
- [ ] Implement key rotation (`kid` support)
- [ ] Add `gobreaker` circuit breaker to BFF
- [ ] Export DB connection pool metrics

---

## Conclusion

**Current State**: The platform is ready for **staging/demo deployment**. Core functionality works correctly after P0 fixes.

**For Production**: Address security items (HSTS, key rotation) and operational items (DLQ, graceful shutdown) before handling real user traffic.

**Estimated Production-Ready ETA**: 1-2 days of focused work.
