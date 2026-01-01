# Phase 1 Implementation Plan - Docker & Middleware

## Goal Description
Establish a local development environment where all services (`auth`, `event`, `join`, `email`, `bff`, `web`) run with a single command (`docker compose up`). Ensure all services handle `X-Request-Id` consistently.

## User Review Required
> [!IMPORTANT]
> I will scaffold a **Go/Chi** `bff-service` independent of the Frontend.
> I will scaffold a **React/Vite** `apps/web`.

## Proposed Changes

### 1. Unified Infrastructure (`docker-compose.yml`)
#### [NEW] [docker-compose.yml](file:///d:/myplayground/self-project/flask-framework/real-time-recsys/docker-compose.yml)
- Extends `compose.infra.yml` (if possible, or merges it).
- Defines services:
    - `auth-service` (Port 8081)
    - `event-service` (Port 8082)
    - `join-service` (Port 8083)
    - `email-service` (No public port)
    - `bff-service` (Port 8080 - **Main Entry**)
    - `web` (Port 3000 - UI)
- All services connect to the default `infra_default` network.

### 2. Dockerization (Add Dockerfile)
Create `Dockerfile` for each service (Multistage Go build).
- `services/auth-service/Dockerfile`
- `services/event-service/Dockerfile`
- `services/join-service/Dockerfile`
- `services/email-service/Dockerfile`

### 3. Service Scaffolding (New Components)
#### [NEW] [services/bff-service/*](file:///d:/myplayground/self-project/flask-framework/real-time-recsys/services/bff-service)
- Initialize a minimal Go Module.
- Implement `/api/healthz` (returns 200 OK).
- Implement RequestID Middleware.

#### [NEW] [apps/web/*](file:///d:/myplayground/self-project/flask-framework/real-time-recsys/apps/web)
- Scaffold a Vite React TS app (`npm create vite@latest`).
- Add minimal `Dockerfile`.

### 4. Middleware Standardization (`X-Request-Id`)
Verify and update `RequestID` middleware in all Go services.
- **Rule**:
    1. Check `X-Request-Id` header.
    2. If missing, generate UUID.
    3. Put in Context.
    4. Put in Response Header.
    5. Log with `request_id` field.

#### [MODIFY] [services/auth-service/internal/transport/http/middleware/request_id.go](file:///d:/myplayground/self-project/flask-framework/real-time-recsys/services/auth-service)
#### [MODIFY] [services/event-service/internal/transport/http/middleware/request_id.go](file:///d:/myplayground/self-project/flask-framework/real-time-recsys/services/event-service)
#### [MODIFY] [services/email-service/internal/infrastructure/web/middleware/request_id.go](file:///d:/myplayground/self-project/flask-framework/real-time-recsys/services/email-service)

## Verification Plan

### Automated
1. **Build & Up**:
    ```bash
    docker compose up --build -d
    ```
2. **Health Check Script**:
    - `curl -i localhost:8080/api/healthz` (BFF) -> 200 OK, header `X-Request-Id` present.
    - `curl -i localhost:3000` (Web) -> 200 OK.
    - `curl -i localhost:8081/healthz` (Auth) -> 200 OK.

### Manual
- Inspect logs: `docker compose logs -f` and verify `request_id` field appears in logs for a single request.
