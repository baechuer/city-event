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

log "Default Go tests"
run_go test ./...

log "Ensure Postgres is running"
run_docker compose up -d postgres
wait_for_compose_health postgres 120

log "Auth Postgres integration tests"
run_go test -tags=integration ./internal/services/auth

if [[ "$skip_runtime_smoke" == true ]]; then
  echo "Runtime smoke skipped."
  exit 0
fi

log "Auth service runtime smoke"
mkdir -p tmp
run_go build -o tmp/auth-service.exe ./cmd/auth-service

port=18082
AUTH_SERVICE_HTTP_ADDR="127.0.0.1:$port" \
POSTGRES_URL="postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable" \
JWT_SECRET="phase-2-smoke-secret" \
  ./tmp/auth-service.exe >tmp/auth-service-smoke.log 2>&1 &
pid=$!
trap 'cleanup_pid "$pid"' EXIT

if ! wait_for_http "http://127.0.0.1:$port/readyz" 20; then
  cat tmp/auth-service-smoke.log || true
  die "auth-service did not become ready."
fi

email="phase2-$(date +%s%N)@example.com"
register_body="$(printf '{"email":"%s","password":"StrongerPass123","displayName":"Phase Two"}' "$email")"
registered="$(curl -fsS -X POST "http://127.0.0.1:$port/v1/auth/register" -H "Content-Type: application/json" --data "$register_body")"
grep -Fq '"accessToken"' <<<"$registered" || die "register did not return an access token."

login_body="$(printf '{"email":"%s","password":"StrongerPass123"}' "$email")"
logged_in="$(curl -fsS -X POST "http://127.0.0.1:$port/v1/auth/login" -H "Content-Type: application/json" --data "$login_body")"
token="$(json_string_field "$logged_in" accessToken)"
[[ -n "$token" ]] || die "login did not return an access token."

me="$(curl -fsS "http://127.0.0.1:$port/v1/auth/me" -H "Authorization: Bearer $token")"
grep -Fq "$email" <<<"$me" || die "me returned wrong user."

logout_status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:$port/v1/auth/logout" -H "Authorization: Bearer $token")"
[[ "$logout_status" == "204" ]] || die "logout returned $logout_status."

after_logout_status="$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/v1/auth/me" -H "Authorization: Bearer $token")"
[[ "$after_logout_status" == "401" ]] || die "revoked token returned $after_logout_status."

cleanup_pid "$pid"
trap - EXIT

echo "Phase 2 verification completed."
