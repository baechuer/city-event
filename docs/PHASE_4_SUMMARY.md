# Phase 4 Summary: BFF Core & Business Logic

**Status: COMPLETED**
**Date: 2026-01-02**

## Overview
Phase 4 focused on establishing the **BFF (Backend-for-Frontend)** as a robust orchestrator. We transitioned from simple proxying to active business logic execution, aggregation, and contract enforcement.

## Key Accomplishments

### 1. Unified API Contract Alignment
- Aligned all `/api/*` endpoints with the [bff-api.md](../docs/bff-api.md) contract.
- Implemented **Unified Error Format** (HTTP status >= 400 returns `{ error: { code, message, request_id } }`).
- Implemented **Pagination Mapping** for `/api/events` and `/api/feed`, ensuring `has_more` flag and `EventCard` view models are provided.

### 2. Event Detail Aggregator
- Implemented `GET /api/events/{id}/view`.
- **Parallel Orchestration**: Fetches from `event-service` and `join-service` concurrently.
- **Structured Degraded Mode**: If `join-service` times out or is unavailable, the response gracefully returns partial data with a `degraded` indicator, allowing the UI to remain functional.
- **ActionPolicy**: Pure logic determines `can_join` and `can_cancel` based on auth status, event state, and capacity.

### 3. Transactional Actions & Idempotency
- Implemented `POST /api/events/{id}/join` and `POST /api/events/{id}/cancel`.
- **Mandatory Idempotency**: Enforces `Idempotency-Key` and `X-Request-Id` headers.
- **Conflict Handling**: Properly handles `already_joined` and `idempotency_key_mismatch` scenarios.
- **Direct Feedback**: Returns the latest `JoinState` directly in the response to avoid unnecessary frontend refetches.

### 4. Technical Quality & Stability
- **Interface-based Refactor**: `EventHandler` now uses interfaces for downstream clients, enabling robust testing.
- **Unit Testing**: Comprehensive test suite for handlers covering aggregation, degradation (timeouts), and resource-not-found scenarios.
- **Cross-Service Fixes**: Resolved all signature mismatches in `join-service` integration tests caused by the repository's new idempotency requirements.

## Files Modified/Created
- `services/bff-service/internal/api/handlers/events.go`
- `services/bff-service/internal/api/handlers/events_test.go`
- `services/bff-service/internal/api/handlers/common.go`
- `services/bff-service/internal/domain/action_policy.go`
- `services/bff-service/internal/downstream/clients.go`
- `services/bff-service/internal/api/router.go`
- `services/join-service/internal/infrastructure/postgres/...` (Test Fixes)

## Verification Results
- [x] **bff-service**: `go test ./...` passes.
- [x] **join-service**: `go test ./...` (including concurrency and outbox) passes.
- [x] **Contract Check**: All responses match `bff-api.md`.

---
*Phase 4 is now closed. The system is ready for Frontend Phase 5 (UI Integration).*
