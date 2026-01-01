# Phase 0 Audit & Recommendation Report

## 1. Executive Summary
**Verdict**: **Strong Endorsement (Recommended)**.
The "Phase 0: freeze API & BFF-first" strategy is highly appropriate for your current project state. It aligns perfectly with the "Frontend First" goal while accommodating the existing "Microservices Backend".

**Key Rationale**:
*   **Decoupling**: Defining `contracts/bff-api.md` now prevents "Backend Rework" later when Frontend requirements change.
*   **Parallelism**: Frontend can build against mocks/contracts while Backend fills in the missing endpoints (like the specific Join status).
*   **Stability**: The "Feed Contract" approach allows you to verify the UI flow immediately using the existing FTS implementation, deferring the complex Redis implementation to Phase 9.

---

## 2. Detailed Component Audit

### 2.1 BFF Definition & Unified Errors
*   **Plan**: Define `docs/bff-api.md` and standard errors (`code`, `message`, `requestId`).
*   **Reasoning**: Microservices often return disparate error formats (e.g., Auth service might return `{ "error": "msg" }`, Join service `{ "err_code": 101 }`).
*   **Trade-off**:
    *   *Pro*: Frontend code is clean (one ErrorHandler).
    *   *Con*: BFF Service needs to write "Error Mapping" logic (translating 500s or timeouts to a standard 502/504).
*   **Alternative**: Frontend handles diverse errors directly. (Not Recommended—leads to spaghetti code).

### 2.2 Pagination (Cursor-based)
*   **Plan**: Use Cursor (`next_cursor`) for Feeds/Lists.
*   **Audit vs Current State**: Your `event-service` *already* implements keyset pagination (`repo_public_keyset.go`). The `join-service` also supports cursors.
*   **Verdict**: **Perfect Alignment**. No backend rework needed for V0.

### 2.3 The "Missing" Endpoint: `GET /join/v1/me/participation/{eventID}`
*   **Plan Question**: "Should we add this?"
*   **Audit Recommendation**: **MUST HAVE (Critical)**.
*   **Reasoning**:
    *   **Frontend Usage**: On the `EventDetail` page, you need to show a button: "Join" OR "Cancel" OR "Waitlist".
    *   **Current Alternative (Bad)**: Frontend calls `/me/joins` (list all my 100 joins) -> iterates to find if `event_id` matches. This is **O(N)** and wasteful.
    *   **Proposed Solution (Good)**: Backend adds an **O(1)** lookup endpoint.
    *   **Cost**: Low. ~1 hour backend work (Repository `GetByEventAndUser` + Handler).
    *   **Benefit**: Massive performance/DX gain for Frontend.

### 2.4 Feed Contract (V0 vs V9)
*   **Plan**: Freeze `GET /api/feed` contract. V0 uses `event-service` (FTS). V1 uses Redis.
*   **Reasoning**: "Fake it until you make it". The Frontend strictly relies on the JSON Contract (`items`, `score`, `cursor`). It doesn't care *how* the score is calculated.
*   **Feasibility**: Your `event-service` already has `ts_rank_cd` (FTS ranking) which produces a "score". Mapping this to the contract is trivial.

---

## 3. Acceptance Criteria & DoD Review
*   [x] `docs/bff-api.md` vs `openapi`: The DoD mentions creating the md doc.
    *   **Refinement**: I recommend adhering to the created **OpenAPI Specs** (`api/openapi/*.yaml`) as the source of truth for the *Service Layer*, but using `bff-api.md` (or a `bff.yaml`) for the *Edge Layer*.
*   [x] **Network Isolation**: The DoD mentions "Browser cannot access microservices".
    *   **Refinement**: This is correct. `docker-compose` network aliases are internal. Frontend running on host (localhost:3000) cannot reach them unless you expose ports. *Recommendation*: Only expose BFF port (e.g. 4000) and BFF proxies traffic.

## 4. Next Actionable Steps (Phase 0 Execution)
1.  **Create `docs/bff-api.md`**: Define the JSON body for the Unified Error and the specific fields for `EventCard`.
2.  **Define the "Join Status" Spec**: Add the `GET /participation` endpoint definition to `join-service` plan (and eventually code).
3.  **Confirm Feed Schema**: Ensure `EventCard` ViewModel (frontend) maps 1:1 to what `event-service` (V0) can provide effectively.
