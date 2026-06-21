#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

base_url="http://127.0.0.1:8080"
frontend_port=18088
start_stack=true
admin_email="${SEED_ADMIN_EMAIL:-admin@cityevents.local}"
admin_password="${SEED_ADMIN_PASSWORD:-AdminPass12345}"
request_timeout=10

usage() {
  cat <<'EOF'
Usage: ./scripts/failure-test-dependencies.sh [options]

Runs GitHub-Actions-only dependency failure evidence:
  - starts the local stack unless --no-start-stack is passed
  - verifies Redis outage fallback/fail-open behavior through the gateway
  - verifies RabbitMQ outage preserves outbox work and recovers after broker restart
  - writes tmp/failure-tests/<run-id>/summary.md

Options:
  --base-url URL          API gateway base URL. Default: http://127.0.0.1:8080
  --frontend-port PORT   Frontend port when the stack is started. Default: 18088
  --no-start-stack       Use an already-running stack
  -h, --help             Show this help

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
    --frontend-port)
      [[ $# -ge 2 ]] || die "--frontend-port requires a value"
      frontend_port="$2"
      shift 2
      ;;
    --no-start-stack)
      start_stack=false
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

cd "$REPO_ROOT"
require_github_actions_evidence_runner "scripts/failure-test-dependencies.sh"
setup_go_cache

run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
run_dir="$REPO_ROOT/tmp/failure-tests/$run_id"
mkdir -p "$run_dir"
results_file="$run_dir/results.tsv"
summary_file="$run_dir/summary.md"
metrics_file="$run_dir/metrics.tsv"
: >"$results_file"
printf 'metric\tvalue\n' >"$metrics_file"

redis_stopped=false
rabbitmq_stopped=false
postgres_stopped=false

record_result() {
  local scenario="$1"
  local status="$2"
  local detail="$3"
  printf '%s\t%s\t%s\n' "$scenario" "$status" "$detail" >>"$results_file"
}

record_metric() {
  local metric="$1"
  local value="$2"
  printf '%s\t%s\n' "$metric" "$value" >>"$metrics_file"
}

millis_now() {
  run_node -e 'console.log(Date.now())'
}

format_seconds() {
  local millis="$1"
  run_node -e 'const ms = Number(process.argv[1]); console.log((ms / 1000).toFixed(3));' "$millis"
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

wait_for_url() {
  local url="$1"
  local timeout_seconds="${2:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

request_json() {
  local expected="$1"
  local method="$2"
  local path="$3"
  local body="$4"
  local outfile="$5"
  shift 5

  local args=(-sS --max-time "$request_timeout" -o "$outfile" -w "%{http_code}" -X "$method" "$base_url$path")
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi
  while [[ $# -gt 0 ]]; do
    args+=(-H "$1")
    shift
  done

  local code
  code="$(curl "${args[@]}" | tr -d '\r')"
  if [[ "$code" != "$expected" ]]; then
    echo "Request failed: $method $path expected $expected got $code" >&2
    cat "$outfile" >&2 || true
    return 1
  fi
  cat "$outfile"
}

request_code() {
  local method="$1"
  local path="$2"
  local body="$3"
  local outfile="$4"
  shift 4

  local args=(-sS --max-time "$request_timeout" -o "$outfile" -w "%{http_code}" -X "$method" "$base_url$path")
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi
  while [[ $# -gt 0 ]]; do
    args+=(-H "$1")
    shift
  done
  curl "${args[@]}" | tr -d '\r'
}

iso_tomorrow() {
  run_node -e 'console.log(new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString())'
}

postgres_query() {
  local name="$1"
  local sql="$2"
  run_docker compose exec -T postgres psql -U cityevents -d cityevents -v ON_ERROR_STOP=1 -c "$sql" >"$run_dir/$name.txt" 2>&1
}

postgres_scalar() {
  local sql="$1"
  run_docker compose exec -T postgres psql -U cityevents -d cityevents -tA -v ON_ERROR_STOP=1 -c "$sql" 2>"$run_dir/postgres-scalar-error.txt" \
    | tr -d '\r' \
    | awk 'NF { gsub(/^[ \t]+|[ \t]+$/, ""); print; exit }'
}

snapshot_compose() {
  local label="$1"
  {
    echo "# Compose Snapshot: $label"
    echo
    echo "- Captured At: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    echo
    echo "## docker compose ps"
    run_docker compose ps || true
    echo
    echo "## outbox status"
    run_docker compose exec -T postgres psql -U cityevents -d cityevents -c "select status, count(*) from outbox_messages group by status order by status;" || true
    echo
    echo "## RabbitMQ queues"
    run_docker compose exec -T rabbitmq rabbitmqctl list_queues name messages messages_ready messages_unacknowledged consumers || true
    echo
    echo "## Redis ping"
    run_docker compose exec -T redis redis-cli ping || true
  } >"$run_dir/compose-$label.md" 2>&1 || true
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
    "$REPO_ROOT/scripts/stop-local.sh" --frontend-port "$frontend_port" --with-infrastructure >/dev/null 2>&1 || true
    if [[ -n "${stack_pid:-}" ]]; then
      wait "$stack_pid" >/dev/null 2>&1 || true
    fi
  fi
}

restore_dependencies() {
  if [[ "$postgres_stopped" == true ]]; then
    run_docker compose start postgres >/dev/null 2>&1 || true
    wait_for_compose_health postgres 120 || true
    postgres_stopped=false
  fi
  if [[ "$rabbitmq_stopped" == true ]]; then
    run_docker compose start rabbitmq >/dev/null 2>&1 || true
    rabbitmq_stopped=false
  fi
  if [[ "$redis_stopped" == true ]]; then
    run_docker compose start redis >/dev/null 2>&1 || true
    redis_stopped=false
  fi
}

write_summary() {
  local exit_code="$1"
  {
    echo "# Dependency Failure Evidence Summary"
    echo
    echo "- Run ID: $run_id"
    echo "- Gateway: $base_url"
    echo "- Exit Code: $exit_code"
    echo "- Evidence Directory: $run_dir"
    echo "- Runner: GitHub Actions only"
    echo "- Metrics TSV: $metrics_file"
    echo
    echo "## Scenario Results"
    echo
    echo "| Scenario | Status | Detail |"
    echo "| --- | --- | --- |"
    local scenario_name scenario_status scenario_detail
    while IFS=$'\t' read -r scenario_name scenario_status scenario_detail; do
      [[ -n "$scenario_name" ]] || continue
      printf '| %s | %s | %s |\n' "$scenario_name" "$scenario_status" "$scenario_detail"
    done <"$results_file"
    echo
    echo "## Recovery Metrics"
    echo
    echo "| Metric | Value |"
    echo "| --- | ---: |"
    local metric_name metric_value
    while IFS=$'\t' read -r metric_name metric_value; do
      [[ "$metric_name" != "metric" && -n "$metric_name" ]] || continue
      printf '| %s | %s |\n' "$metric_name" "$metric_value"
    done <"$metrics_file"
    echo
    echo "## Evidence Scope"
    echo
    echo "This evidence can support broker/cache recovery-path discussion after the Actions artifacts are reviewed. It does not prove production high availability."
  } >"$summary_file"
}

cleanup() {
  local exit_code="$1"
  set +e
  snapshot_compose "cleanup"
  "$REPO_ROOT/scripts/inspect-async-ops.sh" --output-dir "$run_dir/async-ops-inspection" >/dev/null 2>&1 || true
  if [[ "$exit_code" != "0" ]]; then
    record_result "script" "failed" "exit_code=$exit_code"
  fi
  restore_dependencies
  stop_started_stack
  write_summary "$exit_code"
}

trap '__cityevents_exit_code=$?; cleanup "$__cityevents_exit_code"; exit "$__cityevents_exit_code"' EXIT

if [[ "$start_stack" == true ]]; then
  log "Start local stack"
  "$REPO_ROOT/scripts/start-local.sh" --frontend-port "$frontend_port" >"$run_dir/start-local.log" 2>&1 &
  stack_pid=$!
fi

if ! wait_for_url "$base_url/readyz" 120; then
  tail -n 120 "$run_dir/start-local.log" >&2 || true
  die "gateway did not become ready"
fi
snapshot_compose "baseline-start"

log "Admin login"
admin_body="$(printf '{"email":"%s","password":"%s"}' "$admin_email" "$admin_password")"
admin_json="$(request_json 200 POST "/v1/auth/login" "$admin_body" "$run_dir/admin-login.json")"
admin_token="$(json_field "$admin_json" "accessToken")"

register_organizer() {
  local prefix="$1"
  local email="failure-$prefix-$run_id@cityevents.local"
  local body
  body="$(printf '{"email":"%s","password":"FailurePass12345","displayName":"Failure %s"}' "$email" "$prefix")"
  local user_json
  user_json="$(request_json 201 POST "/v1/auth/register" "$body" "$run_dir/$prefix-register.json")"
  organizer_id="$(json_field "$user_json" "user.id")"
  organizer_token="$(json_field "$user_json" "accessToken")"
  request_json 200 PATCH "/v1/auth/users/$organizer_id/role" '{"role":"ORGANIZER"}' "$run_dir/$prefix-promote.json" "Authorization: Bearer $admin_token" >/dev/null
}

create_event() {
  local prefix="$1"
  local capacity="${2:-5}"
  local starts_at
  starts_at="$(iso_tomorrow)"
  local body
  body="$(printf '{"title":"Failure %s %s","description":"Dependency failure evidence","city":"Sydney","venue":"Resilience Lab","startsAt":"%s","capacity":%d}' "$prefix" "$run_id" "$starts_at" "$capacity")"
  local event_json
  event_json="$(request_json 201 POST "/v1/events" "$body" "$run_dir/$prefix-event-create.json" "Authorization: Bearer $organizer_token")"
  event_id="$(json_field "$event_json" "event.id")"
}

wait_for_feed_event() {
  local id="$1"
  local timeout_seconds="${2:-60}"
  local outfile="$3"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    local code
    code="$(request_code GET "/v1/feed/events/$id" "" "$outfile" || true)"
    if [[ "$code" == "200" ]]; then
      return 0
    fi
    sleep 1
  done
  return 1
}

log "Create baseline event"
register_organizer "baseline"
create_event "baseline" 5
baseline_event_id="$event_id"
baseline_projection_start_millis="$(millis_now)"
if wait_for_feed_event "$baseline_event_id" 60 "$run_dir/baseline-feed.json"; then
  baseline_projection_end_millis="$(millis_now)"
  baseline_projection_seconds="$(format_seconds "$(( baseline_projection_end_millis - baseline_projection_start_millis ))")"
  record_metric "baseline_projection_seconds" "$baseline_projection_seconds"
  record_result "baseline-feed-projection" "passed" "event projected before failure scenarios in ${baseline_projection_seconds}s"
else
  die "baseline feed projection did not become available"
fi

log "Redis outage scenario"
run_docker compose stop redis >"$run_dir/redis-stop.txt" 2>&1
redis_stopped=true
feed_code="$(request_code GET "/v1/feed/events/$baseline_event_id" "" "$run_dir/redis-feed-fallback.json")"
event_code="$(request_code GET "/v1/events/$baseline_event_id" "" "$run_dir/redis-gateway-fail-open.json")"
logout_code="$(request_code POST "/v1/auth/logout" "" "$run_dir/redis-auth-logout.json" "Authorization: Bearer $organizer_token")"
record_metric "redis_outage_feed_http_code" "$feed_code"
record_metric "redis_outage_event_http_code" "$event_code"
record_metric "redis_outage_logout_http_code" "$logout_code"
if [[ "$feed_code" == "200" && "$event_code" == "200" && "$logout_code" == "204" ]]; then
  record_result "redis-outage" "passed" "feed fallback=$feed_code gateway fail-open=$event_code logout fallback=$logout_code"
else
  die "redis outage scenario failed: feed=$feed_code event=$event_code logout=$logout_code"
fi
redis_restart_start_millis="$(millis_now)"
run_docker compose start redis >"$run_dir/redis-start.txt" 2>&1
wait_for_compose_health redis 120
redis_restart_end_millis="$(millis_now)"
redis_restart_seconds="$(format_seconds "$(( redis_restart_end_millis - redis_restart_start_millis ))")"
record_metric "redis_restart_health_seconds" "$redis_restart_seconds"
redis_stopped=false
snapshot_compose "after-redis"

log "Postgres outage scenario"
postgres_user_email="postgres-outage-$run_id@cityevents.local"
postgres_event_title="Postgres Outage $run_id"
postgres_event_body="$(printf '{"title":"%s","description":"Postgres outage evidence","city":"Sydney","venue":"Database Lab","startsAt":"%s","capacity":5}' "$postgres_event_title" "$(iso_tomorrow)")"
postgres_register_body="$(printf '{"email":"%s","password":"FailurePass12345","displayName":"Postgres Outage"}' "$postgres_user_email")"
postgres_stop_start_millis="$(millis_now)"
run_docker compose stop postgres >"$run_dir/postgres-stop.txt" 2>&1
postgres_stopped=true
postgres_stop_end_millis="$(millis_now)"
postgres_stop_seconds="$(format_seconds "$(( postgres_stop_end_millis - postgres_stop_start_millis ))")"
record_metric "postgres_stop_seconds" "$postgres_stop_seconds"
postgres_register_code="$(request_code POST "/v1/auth/register" "$postgres_register_body" "$run_dir/postgres-register-while-down.json")"
postgres_event_code="$(request_code POST "/v1/events" "$postgres_event_body" "$run_dir/postgres-event-create-while-down.json" "Authorization: Bearer $organizer_token")"
record_metric "postgres_outage_register_http_code" "$postgres_register_code"
record_metric "postgres_outage_event_create_http_code" "$postgres_event_code"
if [[ "$postgres_register_code" =~ ^5 && "$postgres_event_code" =~ ^5 ]]; then
  record_result "postgres-outage-safe-failure" "passed" "register failed with $postgres_register_code; protected event write failed with $postgres_event_code"
else
  die "postgres outage safe failure expected 5xx responses, got register=$postgres_register_code event=$postgres_event_code"
fi
postgres_restart_start_millis="$(millis_now)"
run_docker compose start postgres >"$run_dir/postgres-start.txt" 2>&1
wait_for_compose_health postgres 120
postgres_restart_end_millis="$(millis_now)"
postgres_restart_seconds="$(format_seconds "$(( postgres_restart_end_millis - postgres_restart_start_millis ))")"
record_metric "postgres_restart_health_seconds" "$postgres_restart_seconds"
postgres_stopped=false
postgres_user_count="$(postgres_scalar "select count(*) from auth_users where email = '$postgres_user_email';" || echo "unavailable")"
postgres_event_count="$(postgres_scalar "select count(*) from events where title = '$postgres_event_title';" || echo "unavailable")"
record_metric "postgres_outage_partial_user_rows" "$postgres_user_count"
record_metric "postgres_outage_partial_event_rows" "$postgres_event_count"
if [[ "$postgres_user_count" == "0" && "$postgres_event_count" == "0" ]]; then
  record_result "postgres-outage-no-partial-writes" "passed" "no partial user/event rows after restore"
else
  die "postgres outage left partial rows: users=$postgres_user_count events=$postgres_event_count"
fi
snapshot_compose "after-postgres"

log "RabbitMQ outage scenario"
register_organizer "rabbitmq"
run_docker compose stop rabbitmq >"$run_dir/rabbitmq-stop.txt" 2>&1
rabbitmq_stopped=true
rabbitmq_write_start_millis="$(millis_now)"
create_event "rabbitmq-outage" 5
rabbitmq_write_end_millis="$(millis_now)"
rabbitmq_write_seconds="$(format_seconds "$(( rabbitmq_write_end_millis - rabbitmq_write_start_millis ))")"
record_metric "rabbitmq_outage_write_seconds" "$rabbitmq_write_seconds"
rabbitmq_event_id="$event_id"
outbox_status="$(postgres_scalar "select status from outbox_messages where aggregate_id = '$rabbitmq_event_id' order by created_at desc limit 1;")"
record_metric "rabbitmq_outage_outbox_initial_status" "${outbox_status:-missing}"
postgres_query "rabbitmq-outbox-status" "select id, aggregate_id, routing_key, status, attempts, available_at, last_error from outbox_messages where aggregate_id = '$rabbitmq_event_id' order by created_at desc;"
if [[ -n "$outbox_status" ]]; then
  record_result "rabbitmq-outage-persistence" "passed" "event accepted while broker down in ${rabbitmq_write_seconds}s; outbox status=$outbox_status"
else
  die "rabbitmq outage did not leave an outbox row for event $rabbitmq_event_id"
fi

rabbitmq_restart_start_millis="$(millis_now)"
run_docker compose start rabbitmq >"$run_dir/rabbitmq-start.txt" 2>&1
wait_for_compose_health rabbitmq 120
rabbitmq_restart_end_millis="$(millis_now)"
rabbitmq_restart_seconds="$(format_seconds "$(( rabbitmq_restart_end_millis - rabbitmq_restart_start_millis ))")"
record_metric "rabbitmq_restart_health_seconds" "$rabbitmq_restart_seconds"
rabbitmq_stopped=false
snapshot_compose "after-rabbitmq-restart"

rabbitmq_projection_start_millis="$(millis_now)"
if wait_for_feed_event "$rabbitmq_event_id" 90 "$run_dir/rabbitmq-feed-after-recovery.json"; then
  rabbitmq_projection_end_millis="$(millis_now)"
  rabbitmq_projection_seconds="$(format_seconds "$(( rabbitmq_projection_end_millis - rabbitmq_projection_start_millis ))")"
  rabbitmq_total_recovery_seconds="$(format_seconds "$(( rabbitmq_projection_end_millis - rabbitmq_restart_start_millis ))")"
  rabbitmq_final_outbox_status="$(postgres_scalar "select status from outbox_messages where aggregate_id = '$rabbitmq_event_id' order by created_at desc limit 1;" || echo "unavailable")"
  outbox_dead_count="$(postgres_scalar "select count(*) from outbox_messages where status = 'DEAD';" || echo "unavailable")"
  record_metric "rabbitmq_projection_recovery_seconds" "$rabbitmq_projection_seconds"
  record_metric "rabbitmq_total_recovery_seconds" "$rabbitmq_total_recovery_seconds"
  record_metric "rabbitmq_outbox_final_status" "$rabbitmq_final_outbox_status"
  record_metric "outbox_dead_count_after_failure_tests" "$outbox_dead_count"
  record_metric "data_loss_count" "0"
  record_result "rabbitmq-recovery" "passed" "event projected after broker restart in ${rabbitmq_projection_seconds}s; total recovery ${rabbitmq_total_recovery_seconds}s"
else
  die "rabbitmq recovery did not project event after broker restart"
fi

snapshot_compose "completed"
echo "Dependency failure evidence passed. Summary: $summary_file"
