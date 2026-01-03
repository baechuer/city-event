# CityEvents Platform: Engineering Showcase

> **Role**: Senior Backend Engineer / Systems Architect  
> **Stack**: Go, PostgreSQL, RabbitMQ, Redis, React + TypeScript, Docker

---

## 1. Executive Summary

CityEvents is a distributed event management platform designed to handle high-concurrency "ticket drop" scenarios. It demonstrates solutions to classic distributed systems challenges:

- **Race-free capacity management** via pessimistic locking
- **Exactly-once message processing** via transactional outbox/inbox patterns
- **Secure session handling** using the "B1" Authentication Pattern

---

## 2. Key Technical Achievements

### 🛡️ Concurrency Control: Zero Over-Booking Guarantee

**Problem**: 1000 users simultaneously trying to join an event with 1 remaining slot.

**Solution**: Pessimistic Locking with strict ordering.

```go
// join-service/internal/infrastructure/postgres/repository.go
// 1. Lock Capacity FIRST (Parent)
err = tx.QueryRow(ctx, `SELECT ... FROM event_capacity WHERE event_id = $1 FOR UPDATE`, eventID)

// 2. Lock Join Record SECOND (Child)
err = tx.QueryRow(ctx, `SELECT ... FROM joins WHERE ... FOR UPDATE`, ...)
```

**Result**: Verified correct under load testing. Deadlock-free due to consistent lock ordering.

---

### ✉️ Reliability: Transactional Outbox Pattern

**Problem**: Ensuring data consistency between Postgres and RabbitMQ without 2PC.

**Solution**: Write business data + outbox entry in same ACID transaction.

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  HTTP Request   │───▶│   Transaction   │───▶│   Outbox Worker │
│  Join Event     │    │  INSERT joins   │    │  Polls outbox   │
│                 │    │  INSERT outbox  │    │  Publishes to   │
│                 │    │  COMMIT         │    │  RabbitMQ       │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

**Benefit**: If RabbitMQ is down, events queue in outbox and are delivered when it recovers.

---

### 🔐 Security: B1 Authentication Pattern

**Problem**: JWT in localStorage is vulnerable to XSS attacks.

**Solution**: Split token storage.

| Token | Storage | Accessible to JS? |
|-------|---------|-------------------|
| Refresh Token | HttpOnly Cookie | ❌ No |
| Access Token | In-Memory Variable | ✅ Yes (but ephemeral) |

**Flow**:
1. Login → Server sets `refresh_token` cookie
2. Frontend calls `/refresh` → Gets access token in response body
3. Access token stored in closure, never persisted
4. Page refresh → Silent re-authentication via cookie

---

## 3. Feed Service: Personalization at Scale

### Scoring Formula
```sql
trend_score = 4.0 * join_users_24h 
            + 2.0 * join_users_7d 
            + 0.5 * view_users_24h 
            + 3.0 / (1 + days_until_start)
```

### Personalization Pipeline
```
Candidates (200) → User Prefs Lookup → Rerank (tag affinity) → Diversity Injection → Top N
```

### Pagination Strategy
- **Keyset Pagination**: O(1) fetches regardless of depth
- **Cursor Format**: `base64(score|timestamp|id)`
- **Verified**: Cursor now correctly passed through personalized feed path

---

## 4. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Browser (React SPA)                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  TanStack Query  │  Token Store  │  Router                │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     BFF Service (:8080)                          │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐                 │
│  │ Auth MW    │  │ Rate Limit │  │ Aggregation │                 │
│  └────────────┘  └────────────┘  └────────────┘                 │
└─────────────────────────────────────────────────────────────────┘
              │              │              │
              ▼              ▼              ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│  Auth Service    │ │  Event Service   │ │  Join Service    │
│  (:8081)         │ │  (:8082)         │ │  (:8083)         │
│  ┌────────────┐  │ │  ┌────────────┐  │ │  ┌────────────┐  │
│  │ PostgreSQL │  │ │  │ PostgreSQL │  │ │  │ PostgreSQL │  │
│  │ (auth_db)  │  │ │  │ (event_db) │  │ │  │ (join_db)  │  │
│  └────────────┘  │ │  └────────────┘  │ │  └────────────┘  │
└──────────────────┘ └──────────────────┘ └──────────────────┘
                              │
                              ▼
                     ┌──────────────────┐
                     │    RabbitMQ      │
                     │  (Topic Exchange)│
                     └──────────────────┘
                              │
                              ▼
                     ┌──────────────────┐
                     │  Email Service   │
                     │  (Consumer)      │
                     └──────────────────┘
```

---

## 5. Quality Gates Achieved

| Gate | Status |
|------|--------|
| Unit Tests | ✅ All services passing |
| Integration Tests | ✅ DB/RabbitMQ integration verified |
| Concurrency Tests | ✅ No race conditions under load |
| Security Headers | ✅ CSP, X-Frame-Options, etc. |
| Readiness Probes | ✅ DB health check included |
| Pagination | ✅ Keyset cursor working for all feed types |

---

## 6. What I Learned

1. **Defense in Depth**: Middleware alone isn't enough. Handler-level validation must be explicit.
2. **Verify Assumptions**: Just because a function accepts a parameter doesn't mean it uses it correctly.
3. **Consistent Locking Order**: The order of `FOR UPDATE` statements matters for deadlock prevention.
4. **Audit Early**: Production readiness audits should happen before feature complete, not after.
