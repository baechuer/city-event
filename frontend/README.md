# CityEvents Frontend

The frontend is a React + TypeScript app built with Vite. It keeps the browser-facing API contract stable through runtime `/config.js`, so the compiled bundle does not hardcode the gateway origin.

Run locally:

```bash
./scripts/start-local.sh
```

Then open:

```text
http://127.0.0.1:18088
```

The app calls the API gateway by default:

```text
http://127.0.0.1:18088 frontend
  -> http://127.0.0.1:8080 api-gateway
  -> internal services
```

The frontend server emits `/config.js` at runtime. By default it sets all API bases to the local gateway. Override the target when serving the frontend:

```bash
CITYEVENTS_API_BASE=http://cityevents.local ./scripts/serve-frontend.sh
```

The app already appends `/v1/...`, so `CITYEVENTS_API_BASE` should be the gateway or ingress origin.

Access tokens are kept in browser memory only. Page reload calls `/v1/auth/refresh`; the refresh token is an HttpOnly cookie set by the auth service.

Refresh-cookie operations use double-submit CSRF protection. Auth-service sets a readable `cityevents_csrf` cookie alongside the HttpOnly refresh cookie, and the frontend sends that value in `X-CSRF-Token` for `/v1/auth/refresh` and `/v1/auth/logout`.

For frontend-only static serving, after separately starting backend services:

```bash
./scripts/serve-frontend.sh
```

Useful frontend commands:

```bash
cd frontend
npm ci
npm run typecheck
npm run test
npm run build
npm run verify
```

Run browser E2E:

```bash
cd frontend
npm ci
npm run e2e
```

The E2E suite starts the local stack through `../scripts/start-local.sh` unless `CITYEVENTS_E2E_SKIP_WEBSERVER=true` is set.
