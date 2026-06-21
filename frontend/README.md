# CityEvents Frontend

The frontend is a React + TypeScript + Vite single-page app for demonstrating
the CityEvents customer and admin workflows. It talks to the public
`api-gateway`; it does not call internal services directly.

## What The UI Demonstrates

- Landing/discovery experience with major event cards and category lanes.
- Event browse page with city/category/search filters.
- Event detail page with live RSVP state.
- Account registration and login.
- Memory-only access token handling plus HttpOnly refresh-cookie recovery.
- Role-aware publish gating for `ORGANIZER` and `ADMIN`.
- Admin role update form.
- Organizer/admin attendee moderation.
- Media upload-intent workflow for live events.
- Runtime API gateway configuration through `/config.js`.

## Runtime Flow

```text
Browser at http://127.0.0.1:18088
  -> /config.js
  -> API base: http://127.0.0.1:8080
  -> api-gateway
  -> auth-service / event-registration-service / feed-service / media-service
```

The compiled bundle does not hardcode the API origin. `frontend/server.mjs` and
`frontend/vite.config.ts` emit `/config.js` at runtime.

Override the gateway/ingress origin when serving the frontend:

```bash
CITYEVENTS_API_BASE=http://cityevents.local ./scripts/serve-frontend.sh
```

The app appends `/v1/...`, so `CITYEVENTS_API_BASE` should be the gateway or
ingress origin, not a service-specific path.

## Auth And Security Behavior

- Login/register responses return a short-lived JWT access token.
- The access token is stored in JavaScript memory only.
- Page reload calls `/v1/auth/refresh`.
- The refresh token is an opaque HttpOnly cookie set by auth-service.
- Refresh and logout send `X-CSRF-Token` from the readable
  `cityevents_csrf` cookie.
- Protected requests send `Authorization: Bearer <access-token>`.
- The frontend never sends `X-User-ID` or `X-User-Role`.
- Role checks in React are only UX. Backend services and the gateway remain
  authoritative.

## Source Structure

```text
frontend/src/main.tsx                    React root
frontend/src/App.tsx                     stateful app shell and API orchestration
frontend/src/appView.ts                  typed ViewModel shared by pages/features
frontend/src/routing.ts                  local route parsing
frontend/src/api.ts                      runtime-configured gateway client
frontend/src/state.ts                    auth/config/status helpers
frontend/src/domain/events.ts            pure event/category helpers
frontend/src/components/                 small shared UI primitives
frontend/src/features/events/            reusable event cards/search controls
frontend/src/features/account/           auth, publish, admin, media, moderation panels
frontend/src/pages/                      route-level page composition
frontend/src/styles.css                  visual system and responsive layout
frontend/e2e/cityevents.spec.mjs         full browser E2E workflow
```

`App.tsx` intentionally owns side effects and mutable app state. Pages and
features receive a typed `ViewModel`. This keeps API orchestration in one place
while avoiding a single giant component file.

No Redux or router library is used yet. Current routing needs are small enough
for local URL parsing; a larger app with nested routes, loaders, and code
splitting could justify React Router or another routing library.

## Run The Full App

From the repository root:

```bash
./scripts/start-local.sh
```

Open:

```text
http://127.0.0.1:18088
```

Seeded local admin:

```text
email: admin@cityevents.local
password: AdminPass12345
```

Stop:

```bash
./scripts/stop-local.sh --with-infrastructure
```

## Frontend-Only Development

Install dependencies:

```bash
cd frontend
npm ci
```

Run Vite dev server:

```bash
npm run dev
```

Serve a built frontend through the production-like local static server:

```bash
./scripts/serve-frontend.sh
```

Use a different frontend port when the default is occupied:

```bash
./scripts/start-local.sh --frontend-port 18188
```

`start-local.sh` adds the chosen frontend origin to local CORS configuration so
browser auth requests work on custom ports.

## Verification

```bash
cd frontend
npm run typecheck
npm run test
npm run build
npm run verify
```

Browser E2E:

```bash
cd frontend
npm run e2e
```

The Playwright E2E starts the full local stack through
`../scripts/start-local.sh` unless `CITYEVENTS_E2E_SKIP_WEBSERVER=true` is set.
It verifies registration, admin promotion, UI login, publishing, joining, and
metrics.

For headed debugging:

```bash
cd frontend
npm run e2e:headed
```

## Product Direction

The UI focuses on the workflows that best demonstrate the backend architecture:
discovery, authentication, role-aware publishing, RSVP state, moderation, and
media upload intent. Natural next additions would be richer account settings,
event image upload UX, a larger admin dashboard, formal accessibility evidence,
analytics, and broader responsive screenshot coverage.
