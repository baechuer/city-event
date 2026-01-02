# Phase 2: Frontend Foundation (React + Vite) Implementation Plan

## Goal
Build a robust, type-safe Single Page Application (SPA) using React that serves as the visual interface for the City Events Platform. The SPA will communicate **exclusively** with the `bff-service` via `/api/*` proxies, adhering to the B1 Authentication Strategy (HttpOnly Refresh Cookie + In-Memory Access Token).

## Technology Stack
- **Framework**: React 18+ (Vite)
- **Language**: TypeScript (Strict)
- **Styling**: TailwindCSS + Shadcn/UI (Radix Primitives)
  - **Theme**: **Emerald** (Primary) + **Zinc** (Base) for a "Healthy/Community" vibe.
  - **Radius**: **0.75rem - 1rem** (Friendly/Meetup style).
- **Routing**: React Router v6
- **State Management**:
  - **Server State**: TanStack Query (React Query)
  - **Auth State**: React Context (Auth **Status** only).
  - **Token Storage**: **Module-level memory store** (Closure) `src/auth/tokenStore.ts`.
- **HTTP Client**: Axios (configured for B1 Auth)

---

## Strict Implementation Rules (Mandatory)

### A. API Types (`src/types/api.ts`)
Must exactly mirror `docs/bff-api.md`.
```typescript
export interface ApiError {
  code: string;
  message: string;
  request_id: string;
  meta?: Record<string, unknown>;
}
// ... CursorEnvelope, EventCard, EventView
```

### B. API Client Architecture (`src/lib/apiClient.ts`)
**Rule:** Axios is **NOT** allowed to toast or redirect directly.
- **BaseURL**: `/api` (Strictly relative)
- **Settings**: `withCredentials: true` (Critical for B1/Phase 3).
- **Auth**: Injects `Authorization: Bearer <token>` from module-level store.
- **Errors**: Normalizes non-2xx responses to `ApiError`. Throws `ApiError`.
- **401 Handling**:
  - **Interceptor**: Only normalize 401 to `ApiError.code = 'UNAUTHENTICATED'`.
  - **NO** Redirect or Logout logic here.

### C. Global Error Handling & Auth Controller (`src/lib/auth.tsx`)
**Rule:** `AuthProvider` handles logout/redirect via Event Bus.
- **Mechanism**: `QueryClient.onError` detects `UNAUTHENTICATED` -> Emits event `auth:logout` -> `AuthProvider` listens.
- **Conditional Logout (Rule A)**:
  - Only trigger logout if `getAccessToken() !== null`.
  - Otherwise, treat as generic error (stay on page).
- **Clean Logout (Rule B)**:
  - `logout()` must call `queryClient.clear()`/`removeQueries()` to wipe sensitive data.
- **Logout Idempotency**:
  - `isLoggingOut` module-level guard.

### D. Global Toast (Rule C)
- **Deduping**: Prevent flooding by deduplicating toasts with same `message` or `code` within N seconds.
- **401 Silence**: Do NOT toast if error code is `UNAUTHENTICATED` (handled by Auth logic).

### E. Phase 2 Known Limitation
- **Page Reload**: Will require re-login because `tokenStore` (memory) is cleared.

---

## Implementation Steps

### Step 1: Types & Core Infrastructure
1. Define Strict Types.
2. Create `src/auth/tokenStore.ts`.
3. Implement `apiClient` (Axios):
   - `withCredentials: true`.
   - **401**: Just return error code `UNAUTHENTICATED`.
4. Configure `queryClient`:
   - `onError`: Emit 'auth:unauthorized' if 401. Else toast (deduped).

### Step 2: Auth Provider & Logout Logic
1. Implement `AuthProvider`:
   - Listens for 'auth:unauthorized'.
   - `logout()`: `isLoggingOut` check -> `queryClient.clear()` -> Clear Token -> Navigate `/login`.
   - Only react if currently authenticated.

### Step 3: UI Foundation & Pages
1. Setup Tailwind + Shadcn/UI (Emerald/Zinc).
2. Create State Components.
3. `/login`, `/events` (Feed), `/events/:id`.

---

## DoD (Definition of Done) - Hard Gates
Phase 2 is **NOT** complete unless:
- [x] `/events` renders real data from `/api/feed`.
- [x] 401 triggers clean Redirect to Login (Idempotent, no loops, only if logged in).
- [x] Logout clears Query Cache (Verified).
- [x] Axios has `withCredentials: true`.
- [x] Toast does not flood on 500s.
