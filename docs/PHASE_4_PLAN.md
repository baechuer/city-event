# Phase 4: BFF Core Business API - Implementation Plan

> **Goal**: Implement the core business logic in the BFF layer (`bff-service`), acting as the orchestrator between the Frontend and Microservices (`event-service`, `join-service`).

## 1. Scope & Objectives

1.  **Events Feed**: `GET /api/events` with frozen paging contract (Strict Time/Filter).
2.  **Event Detail Aggregation**: `GET /api/events/{id}/view` with **Structured Degraded Mode** and **Strict ActionPolicy**.
3.  **Transactional Actions**: `POST /api/events/{id}/join` and `/cancel` with **Strict Idempotency** and **Business Conflict Handling**.
4.  **Feed API (V0)**: `GET /api/feed` (Discovery/Relevance) with identical pagination contract to `/events`.

## 2. API Specifications & Data Flow

### 2.1 Events Feed (`GET /api/events`) & Discovery (`GET /api/feed`)
-   **Contract Freeze**:
    -   **Cursor**: Opaque string (client treats as blob).
    -   **Termination**: `next_cursor: null` implies end.
    -   **Stability**: `start_time ASC, id ASC` (Tie-breaker is mandatory).
-   **Distinction**:
    -   `/events`: Strict chronological/filtering (Stable).
    -   `/feed`: **Discovery/Relevance**. Sort key: `score DESC, start_time ASC, id ASC`. (Explicit Tie-breaker).

### 2.4 Component Architecture
-   **Internal Packages**:
    -   `internal/domain/action_policy.go`: Pure logic.
    -   `internal/downstream/mapping.go`: Centralized error code mapping.
    -   `internal/api/handlers`: Thin HTTP translation.

### 2.2 Event Detail View (`GET /api/events/{id}/view`)
-   **Aggregator Logic**: Parallel fetch `event` and `participation`.
-   **Structured Degraded Mode**:
    -   Field: `degraded: { participation: "timeout" | "unavailable" | "rate_limited" | null }`.
    -   **Rule**: If `degraded` is set, `participation` MUST be `null`.
    -   **Rule**: If `join-service` returns 404/401 -> `participation: { status: "none" }` (NOT null).
-   **ActionPolicy (CRITICAL)**:
    -   **Input**: `Event`, `Participation`, `UserId`.
    -   **Rules (Priority Order)**:
        1.  **Auth Gate**: If `UserId` is empty -> `canJoin=false, reason="auth_required"`.
        2.  **Degraded Gate**: If `degraded` -> `canJoin=false, reason="participation_unavailable"`.
        3.  **Business Logic**: Check capacity, time, status.

### 2.3 Join / Cancel Actions (`POST /api/events/{id}/[join|cancel]`)
-   **Mandatory Headers**: `Idempotency-Key`, `X-Request-Id`.
-   **Response Contract**:
    -   **Always return 200 OK** with body `JoinState` on success (avoids frontend refetch).
    -   Example: `{ "status": "joined", "event_id": "...", "updated_at": "..." }`.
-   **409 Conflict Handling**:
    -   `code == "state_already_reached"` -> Return 200 OK + State.
    -   `code == "idempotency_key_mismatch"` -> Return 409 Conflict.

## 3. Technical Implementation Structure

### 3.1 Components
1.  **DownstreamClients** (`joinClient`, `eventClient`):
    -   **Timeouts (Split Strategy)**:
        -   **Aggregator (View)**: Strict. `participation` ~300-500ms (Fail fast to degrade). `event` ~500-800ms.
        -   **List (Events/Feed)**: Relaxed. ~1500ms (Tolerate cold starts/network jitter).
    -   Error Mapping (Service Error -> Domain Error).
    -   Auth Propagation: `Authorization: Bearer <token>` + `X-Internal-Auth` (Optional).
2.  **Aggregator**:
    -   Composite struct fetching using `errgroup`.
    -   Handling "Soft Failures" (Partial results).
3.  **ActionPolicy (Pure Function)**:
    -   Input: `Event`, `Participation`, `UserId`, `Now`.
    -   Output: `Actions { canJoin, canCancel, reason }`.
    -   *Unit Testable without mocks.*

## 4. Verification Plan

-   [ ] **Manual**:
    -   Verify "Join" button is disabled if simulation of `join-service` timeout occurs.
    -   Verify "Join" allows replay (idempotency) and returns success if already joined.
-   [ ] **Automated Integration**:
    -   **Degradation Test**: Kill `join-service` -> `/view` returns 200 with `degraded` field.
    -   **Idempotency Test**: POST with same key twice -> Both succeed. POST with same key diff payload -> 409.

## 5. Files to Change
-   `services/bff-service/internal/api/handlers/...`
-   `services/bff-service/internal/domain/action_policy.go` (New)
-   `services/bff-service/internal/domain/action_policy_test.go` (New)
-   `docs/bff-api.md` (Update contracts)

