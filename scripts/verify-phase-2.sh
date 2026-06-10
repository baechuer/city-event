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

smoke_curl() {
  if command -v curl.exe >/dev/null 2>&1; then
    curl.exe "$@"
    return
  fi
  curl "$@"
}

smoke_null_target() {
  if command -v curl.exe >/dev/null 2>&1; then
    echo "NUL"
    return
  fi
  echo "/dev/null"
}

wait_for_smoke_http() {
  local url="$1"
  local timeout_seconds="${2:-20}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if smoke_curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.3
  done
  return 1
}

cleanup_smoke() {
  cleanup_pid "${pid:-}"
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "\$target = (Resolve-Path 'tmp/auth-service.exe').Path; Get-CimInstance Win32_Process -Filter \"Name = 'auth-service.exe'\" | Where-Object { \$_.ExecutablePath -eq \$target } | ForEach-Object { Stop-Process -Id \$_.ProcessId -Force }" >/dev/null 2>&1 || true
  fi
}

port=18082
if command -v cmd.exe >/dev/null 2>&1; then
  if command -v cygpath >/dev/null 2>&1; then
    smoke_binary="$(cygpath -w "$REPO_ROOT/tmp/auth-service.exe")"
  elif command -v wslpath >/dev/null 2>&1; then
    smoke_binary="$(wslpath -w "$REPO_ROOT/tmp/auth-service.exe")"
  else
    smoke_binary="$REPO_ROOT/tmp/auth-service.exe"
  fi
  cmd.exe /d /c "set AUTH_SERVICE_HTTP_ADDR=127.0.0.1:$port && set POSTGRES_URL=postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable && set JWT_SECRET=phase-2-smoke-secret && $smoke_binary" >tmp/auth-service-smoke.log 2>&1 &
else
  AUTH_SERVICE_HTTP_ADDR="127.0.0.1:$port" \
  POSTGRES_URL="postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable" \
  JWT_SECRET="phase-2-smoke-secret" \
    ./tmp/auth-service.exe >tmp/auth-service-smoke.log 2>&1 &
fi
pid=$!
trap cleanup_smoke EXIT

if ! wait_for_smoke_http "http://127.0.0.1:$port/readyz" 20; then
  cat tmp/auth-service-smoke.log || true
  die "auth-service did not become ready."
fi

email="phase2-$(date +%s%N)@example.com"
register_body="$(printf '{"email":"%s","password":"StrongerPass123","displayName":"Phase Two"}' "$email")"
registered="$(smoke_curl -fsS -X POST "http://127.0.0.1:$port/v1/auth/register" -H "Content-Type: application/json" --data "$register_body")"
grep -Fq '"accessToken"' <<<"$registered" || die "register did not return an access token."

login_body="$(printf '{"email":"%s","password":"StrongerPass123"}' "$email")"
logged_in="$(smoke_curl -fsS -X POST "http://127.0.0.1:$port/v1/auth/login" -H "Content-Type: application/json" --data "$login_body")"
token="$(json_string_field "$logged_in" accessToken)"
[[ -n "$token" ]] || die "login did not return an access token."

me="$(smoke_curl -fsS "http://127.0.0.1:$port/v1/auth/me" -H "Authorization: Bearer $token")"
grep -Fq "$email" <<<"$me" || die "me returned wrong user."

null_target="$(smoke_null_target)"
logout_status="$(smoke_curl -sS -o "$null_target" -w '%{http_code}' -X POST "http://127.0.0.1:$port/v1/auth/logout" -H "Authorization: Bearer $token" | tr -d '\r')"
[[ "$logout_status" == "204" ]] || die "logout returned $logout_status."

after_logout_status="$(smoke_curl -sS -o "$null_target" -w '%{http_code}' "http://127.0.0.1:$port/v1/auth/me" -H "Authorization: Bearer $token" | tr -d '\r')"
[[ "$after_logout_status" == "401" ]] || die "revoked token returned $after_logout_status."

cleanup_smoke
trap - EXIT

echo "Phase 2 verification completed."
