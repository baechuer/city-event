# Phase 3: Global Token Refresh Summary

## Overview
Phase 3 implemented a robust, "silent" token rotation mechanism to ensure seamless user sessions. The system catches 401 Unauthorized errors, automatically refreshes the short-lived access token using an HttpOnly cookie, and retries the original request.

## Key Achievements

### 1. Robust Token Rotation (`apiClient.ts`)
-   **Silent Refresh**: Intercepts 401 responses and refreshes the token transparently.
-   **RefreshClient Isolation**: Created a dedicated `refreshClient` Axios instance (no interceptors) to perform the refresh call. This strictly prevents infinite recursion where a failed refresh call triggers another refresh attempt.
-   **Single-Flight Mechanism**: Uses a shared module-level promise (`refreshPromise`) to queue concurrent requests during a refresh cycle, preventing "401 storms" (multiple simultaneous refresh calls).

### 2. Safety & Security
-   **Idempotency Gate**: Implemented strict checks to prevent unsafe request replays.
    -   **Safe Methods**: `GET`, `HEAD`, `OPTIONS` are automatically retried.
    -   **Unsafe Methods**: `POST`, `PUT`, `DELETE` are **blocked** from automatic retry unless they carry an `Idempotency-Key` or `X-Idempotency-Key` header.
-   **Loop Prevention**: Explicit checks ensure requests to `/auth/refresh` never trigger the refresh logic themselves.

### 3. State Synchronization
-   **Event Bus**: Implemented `auth:user-update` event to synchronize the React Context (`user` state) immediately after a successful value-returning refresh.
-   **Centralized Logout**: Critical failures during refresh (e.g., token revoked) emit a `auth:logout` event, cleanly clearing state and redirecting the user.

## Verification
Automated browser tests verified:
-   [x] **Safe Replay**: `GET` requests with expired tokens successfully trigger refresh and succeed.
-   [x] **Unsafe Replay Protection**: `POST` requests with expired tokens (and no key) are correctly rejected with 401, preventing unsafe double-submission.
-   [x] **Refresh Integrity**: No infinite loops or valid token overwrites observed.

## Conclusion
The application now supports long-lived sessions with secure, short-lived access tokens. The implementation is "industrial strength," handling concurrency and non-idempotent edges cases correctly.

## Next Step
**Phase 4**: Event Management (Create, Join, Leave).
