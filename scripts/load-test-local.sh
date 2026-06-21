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
feed_timeout_seconds=30
max_p95_seconds="${LOAD_MAX_P95_SECONDS:-10}"
max_p99_seconds="${LOAD_MAX_P99_SECONDS:-10}"
min_success_rate="${LOAD_MIN_SUCCESS_RATE:-100}"
max_5xx="${LOAD_MAX_5XX:-0}"
max_outbox_dead="${LOAD_MAX_OUTBOX_DEAD:-0}"
max_dlq_depth="${LOAD_MAX_DLQ_DEPTH:-0}"
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
  --feed-timeout-seconds N
                         Feed projection catch-up timeout. Default: 30
  --max-p95-seconds N    Fail when join p95 latency is above N. Default: 10
  --max-p99-seconds N    Fail when join p99 latency is above N. Default: 10
  --min-success-rate N   Fail when HTTP success rate is below N percent. Default: 100
  --max-5xx N            Fail when unexpected join 5xx responses exceed N. Default: 0
  --max-outbox-dead N    Fail when outbox DEAD rows exceed N after joins. Default: 0
  --max-dlq-depth N      Fail when RabbitMQ DLQ depth exceeds N after joins. Default: 0
  -h, --help             Show this help

Environment:
  SEED_ADMIN_EMAIL       Admin email. Default: admin@cityevents.local
  SEED_ADMIN_PASSWORD    Admin password. Default: AdminPass12345
  LOAD_MAX_P95_SECONDS   Default for --max-p95-seconds
  LOAD_MAX_P99_SECONDS   Default for --max-p99-seconds
  LOAD_MIN_SUCCESS_RATE  Default for --min-success-rate
  LOAD_MAX_5XX           Default for --max-5xx
  LOAD_MAX_OUTBOX_DEAD   Default for --max-outbox-dead
  LOAD_MAX_DLQ_DEPTH     Default for --max-dlq-depth

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
    --feed-timeout-seconds)
      [[ $# -ge 2 ]] || die "--feed-timeout-seconds requires a value"
      feed_timeout_seconds="$2"
      shift 2
      ;;
    --max-p95-seconds)
      [[ $# -ge 2 ]] || die "--max-p95-seconds requires a value"
      max_p95_seconds="$2"
      shift 2
      ;;
    --max-p99-seconds)
      [[ $# -ge 2 ]] || die "--max-p99-seconds requires a value"
      max_p99_seconds="$2"
      shift 2
      ;;
    --min-success-rate)
      [[ $# -ge 2 ]] || die "--min-success-rate requires a value"
      min_success_rate="$2"
      shift 2
      ;;
    --max-5xx)
      [[ $# -ge 2 ]] || die "--max-5xx requires a value"
      max_5xx="$2"
      shift 2
      ;;
    --max-outbox-dead)
      [[ $# -ge 2 ]] || die "--max-outbox-dead requires a value"
      max_outbox_dead="$2"
      shift 2
      ;;
    --max-dlq-depth)
      [[ $# -ge 2 ]] || die "--max-dlq-depth requires a value"
      max_dlq_depth="$2"
      shift 2
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
[[ "$feed_timeout_seconds" =~ ^[0-9]+$ ]] || die "--feed-timeout-seconds must be an integer"
[[ "$max_5xx" =~ ^[0-9]+$ ]] || die "--max-5xx must be an integer"
[[ "$max_outbox_dead" =~ ^[0-9]+$ ]] || die "--max-outbox-dead must be an integer"
[[ "$max_dlq_depth" =~ ^[0-9]+$ ]] || die "--max-dlq-depth must be an integer"
[[ "$max_p95_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--max-p95-seconds must be a non-negative number"
[[ "$max_p99_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--max-p99-seconds must be a non-negative number"
[[ "$min_success_rate" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--min-success-rate must be a non-negative number"
awk -v n="$min_success_rate" 'BEGIN { exit !(n >= 0 && n <= 100) }' || die "--min-success-rate must be between 0 and 100"
(( users > 0 )) || die "--users must be greater than zero"
(( capacity > 0 )) || die "--capacity must be greater than zero"
(( concurrency > 0 )) || die "--concurrency must be greater than zero"
(( feed_timeout_seconds > 0 )) || die "--feed-timeout-seconds must be greater than zero"

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

millis_now() {
  run_node -e 'console.log(Date.now())'
}

format_seconds() {
  local millis="$1"
  run_node -e 'const ms = Number(process.argv[1]); console.log((ms / 1000).toFixed(3));' "$millis"
}

format_throughput() {
  local count="$1"
  local millis="$2"
  run_node -e '
const count = Number(process.argv[1]);
const millis = Number(process.argv[2]);
const seconds = Math.max(millis / 1000, 0.001);
console.log((count / seconds).toFixed(2));
' "$count" "$millis"
}

latency_percentile() {
  local percentile="$1"
  local count="$2"
  local index=$(( (count * percentile + 99) / 100 ))
  (( index > 0 )) || index=1
  awk '{ print $4 }' "$run_dir/join-results.tsv" | sort -n | awk -v idx="$index" 'NR == idx { print; found = 1; exit } END { if (!found) print "0" }'
}

numeric_gt() {
  awk -v actual="$1" -v limit="$2" 'BEGIN { exit !(actual + 0 > limit + 0) }'
}

numeric_lt() {
  awk -v actual="$1" -v limit="$2" 'BEGIN { exit !(actual + 0 < limit + 0) }'
}

is_integer_value() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

sql_escape_literal() {
  printf "%s" "$1" | sed "s/'/''/g"
}

postgres_scalar() {
  local sql="$1"
  run_docker compose exec -T postgres psql -U cityevents -d cityevents -tA -c "$sql" 2>/dev/null \
    | tr -d '\r' \
    | awk 'NF { gsub(/^[ \t]+|[ \t]+$/, ""); print; exit }'
}

rabbitmq_queue_sum() {
  local pattern="$1"
  run_docker compose exec -T rabbitmq rabbitmqctl list_queues name messages 2>/dev/null \
    | tr -d '\r' \
    | awk -v pattern="$pattern" '$1 ~ pattern && $2 ~ /^[0-9]+$/ { sum += $2 } END { print sum + 0 }'
}

record_gate() {
  local name="$1"
  local status="$2"
  local detail="$3"
  printf '%s\t%s\t%s\n' "$name" "$status" "$detail" >>"$gates_file"
  if [[ "$status" != "PASS" ]]; then
    gate_failures=$((gate_failures + 1))
  fi
}

dependency_snapshot() {
  local label="$1"
  local outfile="$run_dir/dependencies-$label.md"

  {
    echo "# Dependency Snapshot: $label"
    echo
    echo "- Captured At: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    echo

    if ! command -v docker >/dev/null 2>&1; then
      echo "Docker was not available on this runner; dependency snapshot skipped."
      return 0
    fi

    echo "## Docker Compose"
    docker compose ps || true
    echo

    echo "## Docker Stats"
    docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}' || true
    echo

    echo "## Postgres"
    docker compose exec -T postgres psql -U cityevents -d cityevents -c "select count(*) as active_connections from pg_stat_activity;" || true
    docker compose exec -T postgres psql -U cityevents -d cityevents -c "select status, count(*) from outbox_messages group by status order by status;" || true
    docker compose exec -T postgres psql -U cityevents -d cityevents -c "select coalesce(floor(extract(epoch from now() - min(created_at)))::int, 0) as oldest_retryable_outbox_age_seconds from outbox_messages where status in ('PENDING','FAILED');" || true
    docker compose exec -T postgres psql -U cityevents -d cityevents -c "select status, count(*) from event_registrations group by status order by status;" || true
    echo

    echo "## RabbitMQ Queues"
    docker compose exec -T rabbitmq rabbitmqctl list_queues name messages messages_ready messages_unacknowledged consumers || true
    echo

    echo "## Redis Stats"
    docker compose exec -T redis redis-cli info stats \
      | grep -E '^(total_commands_processed|instantaneous_ops_per_sec|total_net_input_bytes|total_net_output_bytes|rejected_connections|expired_keys):' || true
  } >"$outfile" 2>&1 || true
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

dependency_snapshot "before-joins"

join_user() {
  local i="$1"
  local token
  token="$(cat "$run_dir/users/$i.token")"
  local outfile="$run_dir/joins/$i.json"
  local code_time
  if ! code_time="$(curl_cmd -sS --max-time "$request_timeout" -o "$(curl_path "$outfile")" -w "%{http_code} %{time_total}" -X POST "$base_url/v1/events/$event_id/join" \
    -H "Authorization: Bearer $token" \
    -H "Idempotency-Key: load-$run_id-$i" | tr -d '\r')"; then
    if [[ -z "${code_time:-}" ]]; then
      code_time="000 $request_timeout"
    fi
  fi
  local code="${code_time%% *}"
  local duration="${code_time##* }"
  local status="ERROR"
  if [[ "$code" == "200" ]]; then
    status="$(json_field "$(cat "$outfile")" "status" 2>/dev/null || echo "ERROR")"
  fi
  printf '%s\t%s\t%s\t%s\n' "$i" "$code" "$status" "$duration" >"$run_dir/joins/$i.tsv"
}

log "Run concurrent joins users=$users capacity=$capacity concurrency=$concurrency"
start_millis="$(millis_now)"
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
end_millis="$(millis_now)"

cat "$run_dir"/joins/*.tsv >"$run_dir/join-results.tsv"
total_ok="$(awk '$2 == "200" { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
confirmed="$(awk '$2 == "200" && $3 == "CONFIRMED" { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
waitlisted="$(awk '$2 == "200" && $3 == "WAITLISTED" { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
failed="$(( users - total_ok ))"
join_4xx_count="$(awk '$2 ~ /^4/ { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
join_5xx_count="$(awk '$2 ~ /^5/ { count++ } END { print count + 0 }' "$run_dir/join-results.tsv")"
result_count="$(wc -l <"$run_dir/join-results.tsv" | tr -d ' ')"
p50="$(latency_percentile 50 "$result_count")"
p95="$(latency_percentile 95 "$result_count")"
p99="$(latency_percentile 99 "$result_count")"
max_latency="$(awk 'BEGIN { max = 0 } { if ($4 > max) max = $4 } END { printf "%.6f", max }' "$run_dir/join-results.tsv")"
join_duration_millis="$(( end_millis - start_millis ))"
if (( join_duration_millis < 1 )); then
  join_duration_millis=1
fi
duration_seconds="$(format_seconds "$join_duration_millis")"
join_throughput="$(format_throughput "$total_ok" "$join_duration_millis")"
join_success_rate="$(run_node -e 'const ok = Number(process.argv[1]); const total = Number(process.argv[2]); console.log(((ok / total) * 100).toFixed(2));' "$total_ok" "$users")"
join_error_rate="$(run_node -e 'const failed = Number(process.argv[1]); const total = Number(process.argv[2]); console.log(((failed / total) * 100).toFixed(2));' "$failed" "$users")"
http_status_counts="$(awk '{ counts[$2]++ } END { for (code in counts) printf "- %s: %d\n", code, counts[code] }' "$run_dir/join-results.tsv" | sort)"
domain_status_counts="$(awk '{ counts[$3]++ } END { for (status in counts) printf "- %s: %d\n", status, counts[status] }' "$run_dir/join-results.tsv" | sort)"

expected_confirmed="$capacity"
if (( users < capacity )); then
  expected_confirmed="$users"
fi
expected_waitlisted="$(( users - expected_confirmed ))"
capacity_invariant_violations=0
if (( confirmed > capacity )); then
  capacity_invariant_violations="$(( confirmed - capacity ))"
fi

detail_json="$(request_json 200 GET "/v1/events/$event_id" "" "$run_dir/event-detail.json")"
detail_confirmed="$(json_field "$detail_json" "confirmedCount")"

gates_file="$run_dir/pass-fail-gates.tsv"
: >"$gates_file"
gate_failures=0

feed_status="skipped"
feed_catchup_seconds="skipped"
if [[ "$skip_feed_check" == false ]]; then
  log "Wait for eventual feed projection"
  feed_status="missing"
  feed_start_millis="$(millis_now)"
  deadline=$((SECONDS + feed_timeout_seconds))
  while (( SECONDS < deadline )); do
    code="$(curl_cmd -sS --max-time "$request_timeout" -o "$(curl_path "$run_dir/feed-detail.json")" -w "%{http_code}" "$base_url/v1/feed/events/$event_id" | tr -d '\r')"
    if [[ "$code" == "200" ]]; then
      feed_confirmed="$(json_field "$(cat "$run_dir/feed-detail.json")" "event.confirmedCount" 2>/dev/null || echo "")"
      if [[ "$feed_confirmed" == "$expected_confirmed" ]]; then
        feed_status="ok"
        feed_end_millis="$(millis_now)"
        feed_catchup_seconds="$(format_seconds "$(( feed_end_millis - feed_start_millis ))")"
        break
      fi
    fi
    sleep 1
  done
fi

dependency_snapshot "after-joins"

event_id_sql="$(sql_escape_literal "$event_id")"
active_registration_count="$(postgres_scalar "select count(*) from event_registrations where event_id = '$event_id_sql' and status in ('CONFIRMED','WAITLISTED');" || true)"
duplicate_active_user_count="$(postgres_scalar "select count(*) from (select user_id from event_registrations where event_id = '$event_id_sql' and status in ('CONFIRMED','WAITLISTED') group by user_id having count(*) > 1) duplicates;" || true)"
outbox_dead_count_after="$(postgres_scalar "select count(*) from outbox_messages where status = 'DEAD';" || true)"
oldest_retryable_outbox_age_after="$(postgres_scalar "select coalesce(floor(extract(epoch from now() - min(created_at)))::int, 0) from outbox_messages where status in ('PENDING','FAILED');" || true)"
dlq_depth_after="$(rabbitmq_queue_sum '\.dlq$' || true)"
retry_queue_depth_after="$(rabbitmq_queue_sum '\.retry\.[0-9]+$' || true)"

[[ -n "$active_registration_count" ]] || active_registration_count="unavailable"
[[ -n "$duplicate_active_user_count" ]] || duplicate_active_user_count="unavailable"
[[ -n "$outbox_dead_count_after" ]] || outbox_dead_count_after="unavailable"
[[ -n "$oldest_retryable_outbox_age_after" ]] || oldest_retryable_outbox_age_after="unavailable"
[[ -n "$dlq_depth_after" ]] || dlq_depth_after="unavailable"
[[ -n "$retry_queue_depth_after" ]] || retry_queue_depth_after="unavailable"

if [[ "$failed" == "0" ]]; then
  record_gate "join-http-success-count" "PASS" "$total_ok/$users joins returned HTTP 200"
else
  record_gate "join-http-success-count" "FAIL" "$failed join requests failed; see $run_dir/join-results.tsv"
fi
if numeric_lt "$join_success_rate" "$min_success_rate"; then
  record_gate "join-success-rate" "FAIL" "$join_success_rate% below threshold $min_success_rate%"
else
  record_gate "join-success-rate" "PASS" "$join_success_rate% >= $min_success_rate%"
fi
if (( join_5xx_count > max_5xx )); then
  record_gate "unexpected-5xx" "FAIL" "$join_5xx_count responses above threshold $max_5xx"
else
  record_gate "unexpected-5xx" "PASS" "$join_5xx_count responses <= $max_5xx"
fi
if [[ "$confirmed" == "$expected_confirmed" ]]; then
  record_gate "confirmed-count" "PASS" "$confirmed confirmed joins"
else
  record_gate "confirmed-count" "FAIL" "$confirmed confirmed joins, want $expected_confirmed"
fi
if [[ "$waitlisted" == "$expected_waitlisted" ]]; then
  record_gate "waitlisted-count" "PASS" "$waitlisted waitlisted joins"
else
  record_gate "waitlisted-count" "FAIL" "$waitlisted waitlisted joins, want $expected_waitlisted"
fi
if [[ "$detail_confirmed" == "$expected_confirmed" ]]; then
  record_gate "event-detail-confirmed-count" "PASS" "event detail confirmedCount=$detail_confirmed"
else
  record_gate "event-detail-confirmed-count" "FAIL" "event detail confirmedCount=$detail_confirmed, want $expected_confirmed"
fi
if [[ "$capacity_invariant_violations" == "0" ]]; then
  record_gate "capacity-invariant" "PASS" "confirmed count did not exceed capacity"
else
  record_gate "capacity-invariant" "FAIL" "$capacity_invariant_violations confirmations above capacity"
fi
if [[ "$active_registration_count" == "$users" ]]; then
  record_gate "active-registration-count" "PASS" "$active_registration_count active registrations"
else
  record_gate "active-registration-count" "FAIL" "$active_registration_count active registrations, want $users"
fi
if [[ "$duplicate_active_user_count" == "0" ]]; then
  record_gate "duplicate-active-users" "PASS" "0 duplicate active users"
else
  record_gate "duplicate-active-users" "FAIL" "$duplicate_active_user_count duplicate active users"
fi
if [[ "$feed_status" == "skipped" ]]; then
  record_gate "feed-projection-catchup" "PASS" "feed check skipped by flag"
elif [[ "$feed_status" == "ok" ]]; then
  record_gate "feed-projection-catchup" "PASS" "caught up in ${feed_catchup_seconds}s"
else
  record_gate "feed-projection-catchup" "FAIL" "did not reach confirmedCount=$expected_confirmed within ${feed_timeout_seconds}s"
fi
if numeric_gt "$p95" "$max_p95_seconds"; then
  record_gate "join-p95-latency" "FAIL" "$p95 seconds above threshold $max_p95_seconds"
else
  record_gate "join-p95-latency" "PASS" "$p95 seconds <= $max_p95_seconds"
fi
if numeric_gt "$p99" "$max_p99_seconds"; then
  record_gate "join-p99-latency" "FAIL" "$p99 seconds above threshold $max_p99_seconds"
else
  record_gate "join-p99-latency" "PASS" "$p99 seconds <= $max_p99_seconds"
fi
if is_integer_value "$outbox_dead_count_after" && (( outbox_dead_count_after <= max_outbox_dead )); then
  record_gate "outbox-dead-count" "PASS" "$outbox_dead_count_after DEAD rows <= $max_outbox_dead"
else
  record_gate "outbox-dead-count" "FAIL" "$outbox_dead_count_after DEAD rows, threshold $max_outbox_dead"
fi
if is_integer_value "$dlq_depth_after" && (( dlq_depth_after <= max_dlq_depth )); then
  record_gate "rabbitmq-dlq-depth" "PASS" "$dlq_depth_after DLQ messages <= $max_dlq_depth"
else
  record_gate "rabbitmq-dlq-depth" "FAIL" "$dlq_depth_after DLQ messages, threshold $max_dlq_depth"
fi

max_stable_rps_candidate="not-established"
if (( gate_failures == 0 )); then
  max_stable_rps_candidate="$join_throughput"
fi

{
cat <<EOF
# CI Load Evidence Summary

- Run ID: $run_id
- Gateway: $base_url
- Event ID: $event_id
- Users: $users
- Capacity: $capacity
- Join concurrency: $concurrency
- Threshold p95 seconds: $max_p95_seconds
- Threshold p99 seconds: $max_p99_seconds
- Threshold minimum success rate percent: $min_success_rate
- Join duration seconds: $duration_seconds
- Join throughput requests/second: $join_throughput
- Max stable RPS candidate: $max_stable_rps_candidate
- Join HTTP success: $total_ok/$users
- Join success rate percent: $join_success_rate
- Join error rate percent: $join_error_rate
- Join unexpected 4xx count: $join_4xx_count
- Join unexpected 5xx count: $join_5xx_count
- Confirmed joins: $confirmed
- Waitlisted joins: $waitlisted
- Event detail confirmedCount: $detail_confirmed
- Join latency p95 seconds: $p95
- Join latency p99 seconds: $p99
- Diagnostic join latency p50 seconds: $p50
- Diagnostic join latency max seconds: $max_latency
- Feed projection: $feed_status
- Feed projection catch-up seconds: $feed_catchup_seconds
- Active registration count: $active_registration_count
- Duplicate active user count: $duplicate_active_user_count
- Capacity invariant violations: $capacity_invariant_violations
- Outbox dead rows after joins: $outbox_dead_count_after
- Oldest retryable outbox age seconds after joins: $oldest_retryable_outbox_age_after
- RabbitMQ DLQ depth after joins: $dlq_depth_after
- RabbitMQ retry queue depth after joins: $retry_queue_depth_after
- Gate failures: $gate_failures
- Dependency snapshot before joins: $run_dir/dependencies-before-joins.md
- Dependency snapshot after joins: $run_dir/dependencies-after-joins.md
- Result files: $run_dir
EOF

echo
echo "## Pass/Fail Gates"
echo
awk -F '\t' '{ printf "- %s: %s - %s\n", $1, $2, $3 }' "$gates_file"
echo
echo "## Join HTTP Status Counts"
echo
printf '%s\n' "$http_status_counts"
echo
echo "## Join Domain Status Counts"
echo
printf '%s\n' "$domain_status_counts"
} >"$run_dir/summary.md"

cat "$run_dir/summary.md"
echo
if (( gate_failures > 0 )); then
  die "CI load evidence failed with $gate_failures gate failure(s). See $run_dir/summary.md"
fi
echo "CI load evidence passed."
