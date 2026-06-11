#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

base_url="http://127.0.0.1:8080"
users=80
capacity=25
concurrency=20
request_timeout=10
start_stack=false
frontend_port=18088
skip_feed_check=false
skip_refresh_check=false
admin_email="${SEED_ADMIN_EMAIL:-admin@cityevents.local}"
admin_password="${SEED_ADMIN_PASSWORD:-AdminPass12345}"

usage() {
  cat <<'EOF'
Usage: ./scripts/load-test-local.sh [options]

Runs a GitHub Actions-only gateway-level load test:
  - logs in seeded admin
  - registers and promotes one organizer
  - creates one event through api-gateway
  - registers N users
  - concurrently joins all users to the event
  - verifies confirmed/waitlisted counts and event detail invariants
  - verifies eventual feed projection unless skipped

Options:
  --base-url URL          API gateway base URL. Default: http://127.0.0.1:8080
  --users N              Number of attendee users to register and join. Default: 80
  --capacity N           Event capacity. Default: 25
  --concurrency N        Concurrent join workers per batch. Default: 20
  --start-stack          Start ./scripts/start-local.sh for this run and stop app processes afterward
  --frontend-port PORT   Frontend port when --start-stack is used. Default: 18088
  --skip-feed-check      Do not wait for eventual feed projection
  --skip-refresh-check   Do not run refresh-cookie CSRF smoke check
  -h, --help             Show this help

Environment:
  SEED_ADMIN_EMAIL       Admin email. Default: admin@cityevents.local
  SEED_ADMIN_PASSWORD    Admin password. Default: AdminPass12345

This script is blocked on local workstations. Run it through the manual
heavy-evidence GitHub Actions workflow.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      [[ $# -ge 2 ]] || die "--base-url requires a value"
      base_url="${2%/}"
      shift 2
      ;;
    --users)
      [[ $# -ge 2 ]] || die "--users requires a value"
      users="$2"
      shift 2
      ;;
    --capacity)
      [[ $# -ge 2 ]] || die "--capacity requires a value"
      capacity="$2"
      shift 2
      ;;
    --concurrency)
      [[ $# -ge 2 ]] || die "--concurrency requires a value"
      concurrency="$2"
      shift 2
      ;;
    --start-stack)
      start_stack=true
      shift
      ;;
    --frontend-port)
      [[ $# -ge 2 ]] || die "--frontend-port requires a value"
      frontend_port="$2"
      shift 2
      ;;
    --skip-feed-check)
      skip_feed_check=true
      shift
      ;;
    --skip-refresh-check)
      skip_refresh_check=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ "$users" =~ ^[0-9]+$ ]] || die "--users must be an integer"
[[ "$capacity" =~ ^[0-9]+$ ]] || die "--capacity must be an integer"
[[ "$concurrency" =~ ^[0-9]+$ ]] || die "--concurrency must be an integer"
(( users > 0 )) || die "--users must be greater than zero"
(( capacity > 0 )) || die "--capacity must be greater than zero"
(( concurrency > 0 )) || die "--concurrency must be greater than zero"

cd "$REPO_ROOT"
require_github_actions_evidence_runner "scripts/load-test-local.sh"
setup_go_cache

run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
run_dir="$REPO_ROOT/tmp/load-test-local/$run_id"
mkdir -p "$run_dir/users" "$run_dir/joins"

use_windows_curl=false
if command -v curl.exe >/dev/null 2>&1 && command -v wslpath >/dev/null 2>&1; then
  use_windows_curl=true
fi

curl_cmd() {
  if [[ "$use_windows_curl" == true ]]; then
    curl.exe "$@"
  else
    curl "$@"
  fi
}

curl_path() {
  if [[ "$use_windows_curl" == true ]]; then
    wslpath -w "$1"
  else
    printf '%s' "$1"
  fi
}

wait_for_gateway() {
  local timeout_seconds="${1:-120}"
  wait_for_url "$base_url/readyz" "$timeout_seconds"
}

wait_for_url() {
  local url="$1"
  local timeout_seconds="${2:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if curl_cmd -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

json_field() {
  local json="$1"
  local path="$2"
  printf '%s' "$json" | run_node -e '
const path = process.argv[1].split(".");
let raw = "";
process.stdin.on("data", chunk => raw += chunk);
process.stdin.on("end", () => {
  const data = JSON.parse(raw || "{}");
  let value = data;
  for (const key of path) value = value?.[key];
  if (value === undefined || value === null) process.exit(2);
  if (typeof value === "object") console.log(JSON.stringify(value));
  else console.log(String(value));
});
' "$path"
}

request_json() {
  local expected="$1"
  local method="$2"
  local path="$3"
  local body="$4"
  local outfile="$5"
  shift 5

  local args=(-sS --max-time "$request_timeout" -o "$(curl_path "$outfile")" -w "%{http_code}" -X "$method" "$base_url$path")
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi
  while [[ $# -gt 0 ]]; do
    args+=(-H "$1")
    shift
  done

  local code
  code="$(curl_cmd "${args[@]}" | tr -d '\r')"
  if [[ "$code" != "$expected" ]]; then
    echo "Request failed: $method $path expected $expected got $code" >&2
    cat "$outfile" >&2 || true
    return 1
  fi
  cat "$outfile"
}

request_code_with_cookies() {
  local method="$1"
  local path="$2"
  local body="$3"
  local cookie_jar="$4"
  local outfile="$5"
  shift 5

  local args=(-sS --max-time "$request_timeout" -b "$(curl_path "$cookie_jar")" -c "$(curl_path "$cookie_jar")" -o "$(curl_path "$outfile")" -w "%{http_code}" -X "$method" "$base_url$path")
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi
  while [[ $# -gt 0 ]]; do
    args+=(-H "$1")
    shift
  done
  curl_cmd "${args[@]}" | tr -d '\r'
}

csrf_from_cookie_jar() {
  local cookie_jar="$1"
  awk '$0 !~ /^#/ && $6 == "cityevents_csrf" { value = $7 } END { print value }' "$cookie_jar" | tr -d '\r'
}

iso_tomorrow() {
  run_node -e 'console.log(new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString())'
}

stop_started_stack() {
  if [[ "$start_stack" == true ]]; then
    if [[ -n "${stack_pid:-}" ]]; then
      kill -INT "$stack_pid" >/dev/null 2>&1 || true
      local deadline=$((SECONDS + 10))
      while (( SECONDS < deadline )) && kill -0 "$stack_pid" >/dev/null 2>&1; do
        sleep 0.2
      done
    fi
    "$REPO_ROOT/scripts/stop-local.sh" --frontend-port "$frontend_port" >/dev/null 2>&1 || true
    if [[ -n "${stack_pid:-}" ]]; then
      if kill -0 "$stack_pid" >/dev/null 2>&1; then
        kill -TERM "$stack_pid" >/dev/null 2>&1 || true
        sleep 1
      fi
      if kill -0 "$stack_pid" >/dev/null 2>&1; then
        kill -KILL "$stack_pid" >/dev/null 2>&1 || true
      fi
      wait "$stack_pid" >/dev/null 2>&1 || true
    fi
  fi
}

if [[ "$start_stack" == true ]]; then
  log "Start local stack"
  "$REPO_ROOT/scripts/start-local.sh" --frontend-port "$frontend_port" >"$run_dir/start-local.log" 2>&1 &
  stack_pid=$!
  trap 'stop_started_stack' EXIT
  if ! wait_for_gateway 120; then
    tail -n 120 "$run_dir/start-local.log" >&2 || true
    die "local stack did not become ready."
  fi
  for url in \
    "http://127.0.0.1:8081/readyz" \
    "http://127.0.0.1:8082/readyz" \
    "http://127.0.0.1:8083/readyz"; do
    if ! wait_for_url "$url" 60; then
      tail -n 120 "$run_dir/start-local.log" >&2 || true
      die "local service did not become ready: $url"
    fi
  done
  if ! kill -0 "$stack_pid" >/dev/null 2>&1; then
    tail -n 120 "$run_dir/start-local.log" >&2 || true
    die "local stack exited unexpectedly."
  fi
elif ! wait_for_gateway 5; then
  die "gateway is not ready at $base_url. Run ./scripts/start-local.sh first or pass --start-stack."
fi

log "Admin login"
admin_body="$(printf '{"email":"%s","password":"%s"}' "$admin_email" "$admin_password")"
admin_json="$(request_json 200 POST "/v1/auth/login" "$admin_body" "$run_dir/admin-login.json")"
admin_token="$(json_field "$admin_json" "accessToken")"

if [[ "$skip_refresh_check" == false ]]; then
  log "Refresh cookie CSRF smoke"
  cookie_jar="$run_dir/admin-cookies.txt"
  : >"$cookie_jar"
  login_code="$(request_code_with_cookies POST "/v1/auth/login" "$admin_body" "$cookie_jar" "$run_dir/admin-cookie-login.json")"
  [[ "$login_code" == "200" ]] || die "cookie login returned $login_code"
  missing_csrf_code="$(request_code_with_cookies POST "/v1/auth/refresh" "" "$cookie_jar" "$run_dir/refresh-missing-csrf.json")"
  [[ "$missing_csrf_code" == "403" ]] || die "refresh without CSRF returned $missing_csrf_code, want 403"
  csrf="$(csrf_from_cookie_jar "$cookie_jar")"
  [[ -n "$csrf" ]] || die "login did not set cityevents_csrf cookie"
  refresh_code="$(request_code_with_cookies POST "/v1/auth/refresh" "" "$cookie_jar" "$run_dir/refresh-ok.json" "X-CSRF-Token: $csrf")"
  [[ "$refresh_code" == "200" ]] || die "refresh with CSRF returned $refresh_code, want 200"
fi

log "Register organizer and promote role"
organizer_email="load-organizer-$run_id@cityevents.local"
organizer_body="$(printf '{"email":"%s","password":"LoadPass12345","displayName":"Load Organizer"}' "$organizer_email")"
organizer_json="$(request_json 201 POST "/v1/auth/register" "$organizer_body" "$run_dir/organizer-register.json")"
organizer_id="$(json_field "$organizer_json" "user.id")"
organizer_token="$(json_field "$organizer_json" "accessToken")"
request_json 200 PATCH "/v1/auth/users/$organizer_id/role" '{"role":"ORGANIZER"}' "$run_dir/organizer-promote.json" "Authorization: Bearer $admin_token" >/dev/null

log "Create load-test event"
starts_at="$(iso_tomorrow)"
event_body="$(printf '{"title":"Load Test %s","description":"Gateway load test","city":"Sydney","venue":"Town Hall","startsAt":"%s","capacity":%d}' "$run_id" "$starts_at" "$capacity")"
event_json="$(request_json 201 POST "/v1/events" "$event_body" "$run_dir/event-create.json" "Authorization: Bearer $organizer_token")"
event_id="$(json_field "$event_json" "event.id")"
[[ -n "$event_id" ]] || die "event create did not return event id"

log "Register $users attendee users"
for i in $(seq 1 "$users"); do
  email="load-user-$run_id-$i@cityevents.local"
  body="$(printf '{"email":"%s","password":"LoadPass12345","displayName":"Load User %d"}' "$email" "$i")"
  user_json="$(request_json 201 POST "/v1/auth/register" "$body" "$run_dir/users/$i.json")"
  json_field "$user_json" "accessToken" >"$run_dir/users/$i.token"
done

join_user() {
  local i="$1"
  local token
  token="$(cat "$run_dir/users/$i.token")"
  local outfile="$run_dir/joins/$i.json"
  local code_time
  code_time="$(curl_cmd -sS --max-time "$request_timeout" -o "$(curl_path "$outfile")" -w "%{http_code} %{time_total}" -X POST "$base_url/v1/events/$event_id/join" \
    -H "Authorization: Bearer $token" \
    -H "Idempotency-Key: load-$run_id-$i" | tr -d '\r')"
  local code="${code_time%% *}"
  local duration="${code_time##* }"
  local status="ERROR"
  if [[ "$code" == "200" ]]; then
    status="$(json_field "$(cat "$outfile")" "status" 2>/dev/null || echo "ERROR")"
  fi
  printf '%s\t%s\t%s\t%s\n' "$i" "$code" "$status" "$duration" >"$run_dir/joins/$i.tsv"
}

log "Run concurrent joins users=$users capacity=$capacity concurrency=$concurrency"
start_epoch="$(date -u '+%s')"
batch=()
for i in $(seq 1 "$users"); do
  join_user "$i" &
  batch+=("$!")
  if (( ${#batch[@]} >= concurrency )); then
    for pid in "${batch[@]}"; do
      wait "$pid"
    done
    batch=()
  fi
done
for pid in "${batch[@]}"; do
  wait "$pid"
done
end_epoch="$(date -u '+%s')"

cat "$run_dir"/joins/*.tsv >"$run_dir/join-results.tsv"
total_ok="$(awk '$2 == "200" { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
confirmed="$(awk '$2 == "200" && $3 == "CONFIRMED" { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
waitlisted="$(awk '$2 == "200" && $3 == "WAITLISTED" { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
failed="$(( users - total_ok ))"
result_count="$(wc -l <"$run_dir/join-results.tsv" | tr -d ' ')"
p95_index=$(( (result_count * 95 + 99) / 100 ))
p95="$(awk '{ print $4 }' "$run_dir/join-results.tsv" | sort -n | awk -v idx="$p95_index" 'NR == idx { print; found = 1; exit } END { if (!found) print "0" }')"
max_latency="$(awk 'BEGIN { max = 0 } { if ($4 > max) max = $4 } END { printf "%.6f", max }' "$run_dir/join-results.tsv")"

expected_confirmed="$capacity"
if (( users < capacity )); then
  expected_confirmed="$users"
fi
expected_waitlisted="$(( users - expected_confirmed ))"

detail_json="$(request_json 200 GET "/v1/events/$event_id" "" "$run_dir/event-detail.json")"
detail_confirmed="$(json_field "$detail_json" "confirmedCount")"

if [[ "$failed" != "0" ]]; then
  die "$failed join requests failed. See $run_dir/join-results.tsv"
fi
if [[ "$confirmed" != "$expected_confirmed" ]]; then
  die "confirmed joins = $confirmed, want $expected_confirmed"
fi
if [[ "$waitlisted" != "$expected_waitlisted" ]]; then
  die "waitlisted joins = $waitlisted, want $expected_waitlisted"
fi
if [[ "$detail_confirmed" != "$expected_confirmed" ]]; then
  die "event detail confirmedCount = $detail_confirmed, want $expected_confirmed"
fi

feed_status="skipped"
if [[ "$skip_feed_check" == false ]]; then
  log "Wait for eventual feed projection"
  feed_status="missing"
  deadline=$((SECONDS + 30))
  while (( SECONDS < deadline )); do
    code="$(curl_cmd -sS --max-time "$request_timeout" -o "$(curl_path "$run_dir/feed-detail.json")" -w "%{http_code}" "$base_url/v1/feed/events/$event_id" | tr -d '\r')"
    if [[ "$code" == "200" ]]; then
      feed_confirmed="$(json_field "$(cat "$run_dir/feed-detail.json")" "event.confirmedCount" 2>/dev/null || echo "")"
      if [[ "$feed_confirmed" == "$expected_confirmed" ]]; then
        feed_status="ok"
        break
      fi
    fi
    sleep 1
  done
  [[ "$feed_status" == "ok" ]] || die "feed projection did not reach confirmedCount=$expected_confirmed within 30s"
fi

duration_seconds="$(( end_epoch - start_epoch ))"
cat >"$run_dir/summary.md" <<EOF
# Local Load Test Summary

- Run ID: $run_id
- Gateway: $base_url
- Event ID: $event_id
- Users: $users
- Capacity: $capacity
- Join concurrency: $concurrency
- Join duration seconds: $duration_seconds
- Join HTTP success: $total_ok/$users
- Confirmed joins: $confirmed
- Waitlisted joins: $waitlisted
- Event detail confirmedCount: $detail_confirmed
- Join latency p95 seconds: $p95
- Join latency max seconds: $max_latency
- Feed projection: $feed_status
- Result files: $run_dir
EOF

cat "$run_dir/summary.md"
echo
echo "Local load test passed."
