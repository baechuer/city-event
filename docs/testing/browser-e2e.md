# Browser E2E Testing

## Purpose

The Playwright suite verifies the browser-facing CityEvents workflow against the real local stack.

It currently covers:

- frontend runtime API configuration from `/config.js`
- seeded admin login through API setup
- organizer role promotion through the gateway
- organizer login through the browser UI
- event publishing through the browser UI
- direct live event detail navigation before the feed projection catches up
- attendee login through the browser UI
- attendee event join through the browser UI
- metrics endpoint exposure for the upgraded request histogram

## Run

Install frontend dependencies:

```bash
cd frontend
npm install
```

Run the browser E2E suite. This starts the local stack through `../scripts/start-local.sh`:

```bash
npm run e2e
```

Run headed mode for debugging:

```bash
npm run e2e:headed
```

If the local stack is already running, skip Playwright's web server launcher:

```bash
CITYEVENTS_E2E_SKIP_WEBSERVER=true npm run e2e
```

## Configuration

| Variable | Purpose | Default |
| --- | --- | --- |
| `CITYEVENTS_FRONTEND_PORT` | frontend port used by Playwright web server | `18088` |
| `CITYEVENTS_E2E_BASE_URL` | browser base URL | `http://127.0.0.1:18088` |
| `CITYEVENTS_API_BASE` | gateway API base used by setup calls | `http://127.0.0.1:8080` |
| `CITYEVENTS_E2E_ADMIN_EMAIL` | seeded admin email | `admin@cityevents.local` |
| `CITYEVENTS_E2E_ADMIN_PASSWORD` | seeded admin password | `AdminPass12345` |

## Artifacts

On failure, Playwright keeps traces and screenshots under:

- `frontend/test-results/`
- `frontend/playwright-report/`

These directories are ignored by Git.

## Limitations

This is an end-to-end browser workflow test, not a production load test. It should be read together with:

- `scripts/load-test-local.sh` for local concurrency/capacity checks
- `scripts/verify-phase-13.sh` for the phase gate
- `docs/testing/load-testing.md` for load-test claim boundaries
