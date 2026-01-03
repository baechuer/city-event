# Technical Design Decisions & Tradeoffs

This document explains the key architectural and implementation decisions made for each component of the CityEvents platform, including alternatives considered and rationale for the chosen approach.

---

## 1. Auth Service

### Decision: JWT vs Session-Based Authentication

| Approach | Pros | Cons |
|----------|------|------|
| **Session (Chosen for Refresh)** | Revocable, server-controlled | Requires Redis/DB for session store |
| **JWT (Chosen for Access)** | Stateless, scalable | Cannot revoke without blocklist |
| **Hybrid (What We Do)** | Best of both | More complexity |

**Our Implementation**:
- **Refresh Token**: Stored in Redis with TTL. Can be revoked instantly.
- **Access Token**: JWT with short TTL (15min). Stateless verification.
- **Why Hybrid?**: We get the scalability of JWT for frequent API calls + the security of revocable refresh tokens.

**Alternative Considered**: Pure JWT with `jti` blocklist. Rejected because maintaining a distributed blocklist adds complexity and latency.

---

### Decision: B1 Pattern (HttpOnly Cookie + In-Memory Access Token)

| Storage | XSS Risk | CSRF Risk | Usability |
|---------|----------|-----------|-----------|
| **localStorage** | ❌ High | ✅ None | ✅ Easy |
| **HttpOnly Cookie (both tokens)** | ✅ None | ❌ High | ⚠️ Complex CSRF handling |
| **B1 Hybrid (Chosen)** | ⚠️ Low (in-memory only) | ✅ Minimal (refresh via cookie) | ✅ Good |

**Why B1?**: The access token lives only in JavaScript memory (not localStorage), making it inaccessible to most XSS attacks. The refresh token is HttpOnly, protecting it completely.

**Alternative Considered**: `SameSite=Strict` cookies for both tokens. Rejected because it breaks cross-origin API calls and complicates the BFF pattern.

---

### Decision: OAuth2 Provider Integration

**Chosen**: Google OAuth2 with state parameter validation.

**Alternative**: OIDC with `id_token` validation. Not implemented due to time constraints, but the infrastructure supports adding it.

---

## 2. Event Service

### Decision: Event Lifecycle State Machine

```
DRAFT → PUBLISHED → CANCELED
         ↓
      (via join-service events)
```

**Why Explicit States?**:
- Prevents accidental modification of published events
- Clear audit trail
- Capacity immutability enforced after publish

**Alternative Considered**: Soft delete / `is_active` flag. Rejected because it loses semantic meaning and complicates query logic.

---

### Decision: Transactional Outbox Pattern

| Pattern | Consistency | Complexity | Performance |
|---------|-------------|------------|-------------|
| **Dual Write** | ❌ Inconsistent | Low | ✅ Fast |
| **2PC (XA)** | ✅ Strong | High | ❌ Slow |
| **Outbox (Chosen)** | ✅ Eventual | Medium | ✅ Good |
| **CDC (Debezium)** | ✅ Eventual | High (ops) | ✅ Good |

**Why Outbox?**: Provides "at-least-once" delivery guarantee with local transaction semantics. No external coordination required.

**Alternative Considered**: Debezium CDC. More robust but adds operational complexity (Kafka, connectors). Overkill for this scale.

---

### Decision: Separate Databases per Service

**Why Polyglot Persistence?**:
- Domain isolation: `event_db` has no foreign keys to `auth_db`
- Independent scaling: Can move hot tables without affecting others
- Demonstrates microservices principle

**Alternative Considered**: Shared database with schema isolation. Viable at small scale, but defeats the purpose of demonstrating microservices patterns.

**Tradeoff Accepted**: No referential integrity across services. Handled via eventual consistency and application-level validation.

---

## 3. Join Service

### Decision: Pessimistic Locking (FOR UPDATE)

| Approach | Consistency | Performance | Complexity |
|----------|-------------|-------------|------------|
| **Optimistic (Version Check)** | ⚠️ Retry needed | ✅ High throughput | Low |
| **Pessimistic (Chosen)** | ✅ Guaranteed | ⚠️ Lock contention | Medium |
| **Queue-based (Redis BLPOP)** | ✅ Serialized | ⚠️ Single-threaded | Low |

**Why Pessimistic?**: For a "ticket drop" scenario where contention is expected, pessimistic locking is more predictable. Optimistic locking would cause retry storms.

**Lock Order**: Always `event_capacity` before `joins` to prevent deadlocks.

---

### Decision: Idempotency Keys vs Optimistic Locking

**Chosen**: Client-generated `Idempotency-Key` header tracked in database.

**Why?**: Network retries are common (user double-clicks, connection drops). Without idempotency, a retry could double-book.

**Alternative Considered**: Database unique constraint on `(event_id, user_id)`. This prevents duplicate joins but doesn't prevent duplicate API calls from creating unnecessary load. Idempotency keys allow early-exit.

---

### Decision: Waitlist Implementation

**Chosen**: Status-based (`active`, `waitlisted`, `canceled`) with ordered queue.

**Alternative**: Separate waitlist table. Rejected to avoid coordinated updates across tables.

**Promotion Logic**: On cancellation, first waitlisted user (by `created_at`) is promoted.

---

## 4. Feed Service

### Decision: Trending Score Calculation

```sql
score = 4.0 * join_users_24h 
      + 2.0 * join_users_7d 
      + 0.5 * view_users_24h 
      + 3.0 / (1 + days_until_start)
```

**Why These Weights?**:
- Joins are high-intent signals (weighted 4x vs views)
- Recency decay for upcoming events
- Simple, explainable formula (no ML black box)

**Alternative Considered**: ML-based ranking (collaborative filtering). Requires training data and infrastructure. Deferred to v2.

---

### Decision: Keyset Pagination vs Offset

| Approach | Performance | Consistency | Complexity |
|----------|-------------|-------------|------------|
| **Offset** | ❌ O(N) with depth | ❌ Items can shift | Low |
| **Keyset (Chosen)** | ✅ O(1) | ✅ Stable | Medium |

**Cursor Format**: `base64(scoreBits|unixNano|eventID)`

**Why Hex for Score?**: Float64 → bits → hex ensures exact roundtrip. Parsing floats from strings loses precision.

---

### Decision: Personalization via Reranking

**Chosen**: Fetch top 200 trending → Apply user preference boost → Return top N.

**Limitation**: Reranking changes order, which complicates cursor-based pagination for deep scrolling.

**Future Improvement**: Pre-compute personalized feeds per user in Redis (fan-out on write).

---

## 5. BFF Service

### Decision: Backend for Frontend Pattern

**Why BFF?**:
1. **Aggregation**: `/events/:id/view` combines event + join status + actions
2. **Protocol Translation**: Internal services use internal contracts; BFF presents clean public API
3. **Security Boundary**: Microservices not exposed to internet

**Alternative Considered**: Direct microservice calls from frontend. Rejected due to CORS complexity and exposed internal APIs.

---

### Decision: Rate Limiting Strategy

| Level | Tool | Limit |
|-------|------|-------|
| **BFF** | `httprate` (Redis-backed) | 100 req/min per IP |
| **Auth endpoints** | Stricter limits | 10 req/min for login |

**Why Redis-backed?**: Consistent limits across BFF replicas.

---

## 6. Frontend (React)

### Decision: TanStack Query for Data Fetching

**Why?**:
- Automatic cache management
- Background refetching
- Optimistic updates for mutations
- Query invalidation on logout

**Alternative Considered**: Redux + RTK Query. More boilerplate, but similar functionality.

---

### Decision: In-Memory Token Storage

**Implementation**: Module-scope variable in `tokenStore.ts`

**Why Not Closure?**: Simpler to debug and integrate with React context.

**Risk Accepted**: XSS can theoretically access module scope. Mitigated by:
- No `dangerouslySetInnerHTML`
- CSP headers
- Short access token TTL

---

### Decision: UI Framework (shadcn/ui)

**Why shadcn?**:
- Copy-paste components (no dependency lock-in)
- Radix primitives for accessibility
- Tailwind integration

**Alternative Considered**: Material UI. Rejected due to larger bundle size and opinionated styling.

---

## Summary: Key Tradeoffs Accepted

| Decision | Benefit | Tradeoff Accepted |
|----------|---------|-------------------|
| Microservices | Domain isolation, independent scaling | Operational complexity, no cross-service transactions |
| JWT + Session Hybrid | Stateless + Revocable | Two token types to manage |
| Pessimistic Locking | Guaranteed consistency | Lock contention under high load |
| Transactional Outbox | Reliable messaging | Eventually consistent (not instant) |
| In-Memory Token | XSS protection vs localStorage | Need silent refresh on page load |
| Keyset Pagination | O(1) performance | More complex cursor handling |

---

*This document serves as architectural decision records (ADRs) for the CityEvents platform.*
