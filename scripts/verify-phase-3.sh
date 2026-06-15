#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

skip_runtime_smoke=false
for arg in "$@"; do
  case "$arg" in
    --skip-runtime-smoke) skip_runtime_smoke=true ;;
    *) die "unknown argument: $arg" ;;
  esac
done

cd "$REPO_ROOT"
setup_go_cache

postgres_url="postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
export EVENT_REGISTRATION_TEST_DATABASE_URL="${EVENT_REGISTRATION_TEST_DATABASE_URL:-$postgres_url}"

log "Default Go tests"
run_go test ./...

log "Ensure Postgres is running"
run_docker compose up -d postgres
wait_for_compose_health postgres 120

log "Integration tests"
run_go test -count=1 -tags=integration ./internal/services/eventregistration

if [[ "$skip_runtime_smoke" == true ]]; then
  echo "Runtime smoke skipped."
  exit 0
fi

log "Event-registration service runtime smoke"
mkdir -p tmp
run_go build -o tmp/event-registration-service.exe ./cmd/event-registration-service

cleanup_smoke() {
  cleanup_pid "${pid:-}"
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "\$target = (Resolve-Path 'tmp/event-registration-service.exe').Path; Get-CimInstance Win32_Process -Filter \"Name = 'event-registration-service.exe'\" | Where-Object { \$_.ExecutablePath -eq \$target } | ForEach-Object { Stop-Process -Id \$_.ProcessId -Force }" >/dev/null 2>&1 || true
  fi
}

port=18083
if command -v cmd.exe >/dev/null 2>&1; then
  smoke_binary="$(windows_binary_path "$REPO_ROOT/tmp/event-registration-service.exe")"
  cmd.exe /d /c "set EVENT_REGISTRATION_SERVICE_HTTP_ADDR=127.0.0.1:$port && set POSTGRES_URL=$postgres_url && $smoke_binary" >tmp/event-registration-service-smoke.log 2>&1 &
else
  EVENT_REGISTRATION_SERVICE_HTTP_ADDR="127.0.0.1:$port" \
  POSTGRES_URL="$postgres_url" \
    ./tmp/event-registration-service.exe >tmp/event-registration-service-smoke.log 2>&1 &
fi
pid=$!
trap cleanup_smoke EXIT

if ! wait_for_http "http://127.0.0.1:$port/readyz" 20; then
  cat tmp/event-registration-service-smoke.log || true
  die "event-registration-service did not become ready."
fi

starts_at="$(date -u -d '+1 day' '+%Y-%m-%dT%H:%M:%SZ')"
create_body="$(printf '{"title":"Phase 3 Smoke Event","description":"Runtime smoke event","city":"Sydney","venue":"Town Hall","startsAt":"%s","capacity":1}' "$starts_at")"
created="$(http_curl -fsS -X POST "http://127.0.0.1:$port/v1/events" -H "Content-Type: application/json" -H "X-User-ID: phase3-organizer" -H "X-User-Role: ORGANIZER" --data "$create_body")"
event_id="$(json_string_field "$created" id)"
[[ -n "$event_id" ]] || die "create event did not return an event ID."

join_one="$(http_curl -fsS -X POST "http://127.0.0.1:$port/v1/events/$event_id/join" -H "X-User-ID: phase3-user-1")"
grep -Fq '"status":"CONFIRMED"' <<<"$join_one" || die "first join did not return CONFIRMED."

join_two="$(http_curl -fsS -X POST "http://127.0.0.1:$port/v1/events/$event_id/join" -H "X-User-ID: phase3-user-2")"
grep -Fq '"status":"WAITLISTED"' <<<"$join_two" || die "second join did not return WAITLISTED."

cancel_one="$(http_curl -fsS -X DELETE "http://127.0.0.1:$port/v1/events/$event_id/join" -H "X-User-ID: phase3-user-1")"
grep -Fq '"userId":"phase3-user-2"' <<<"$cancel_one" || die "cancel did not promote phase3-user-2."

status_two="$(http_curl -fsS "http://127.0.0.1:$port/v1/events/$event_id/join" -H "X-User-ID: phase3-user-2")"
grep -Fq '"status":"CONFIRMED"' <<<"$status_two" || die "promoted user status did not return CONFIRMED."

detail="$(http_curl -fsS "http://127.0.0.1:$port/v1/events/$event_id" -H "X-User-ID: phase3-user-2")"
grep -Fq '"confirmedCount":1' <<<"$detail" || die "event detail did not return confirmed count."
grep -Fq '"viewerJoinStatus":"CONFIRMED"' <<<"$detail" || die "event detail did not return viewer status."

cleanup_smoke
trap - EXIT

echo "Phase 3 verification completed."
