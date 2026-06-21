#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

base_url="http://127.0.0.1:8080"
frontend_port=18088
profiles="${LOAD_SWEEP_PROFILES:-warmup:25:15:25,baseline:50:25:50,100-concurrent:100:50:100,150-concurrent:150:75:150,200-concurrent:200:100:200}"
required_profile="${LOAD_SWEEP_REQUIRED_PROFILE:-100-concurrent}"
max_p95_seconds="${LOAD_MAX_P95_SECONDS:-10}"
max_p99_seconds="${LOAD_MAX_P99_SECONDS:-10}"
min_success_rate="${LOAD_MIN_SUCCESS_RATE:-100}"
max_5xx="${LOAD_MAX_5XX:-0}"
max_outbox_dead="${LOAD_MAX_OUTBOX_DEAD:-0}"
max_dlq_depth="${LOAD_MAX_DLQ_DEPTH:-0}"
feed_timeout_seconds="${LOAD_FEED_TIMEOUT_SECONDS:-30}"

usage() {
  cat <<'EOF'
Usage: ./scripts/load-sweep-local.sh [options]

Runs a GitHub Actions-only load sweep for max stable RPS and saturation evidence.
Each profile runs scripts/load-test-local.sh against the same started local stack.

Options:
  --base-url URL             API gateway base URL. Default: http://127.0.0.1:8080
  --frontend-port PORT       Frontend port for the local stack. Default: 18088
  --profiles LIST            Comma-separated name:users:capacity:concurrency list.
  --required-profile NAME    Profile that must pass for this sweep to pass.
                             Default: 100-concurrent
  --max-p95-seconds N        Pass-through load gate. Default: 10
  --max-p99-seconds N        Pass-through load gate. Default: 10
  --min-success-rate N       Pass-through load gate. Default: 100
  --max-5xx N                Pass-through load gate. Default: 0
  --max-outbox-dead N        Pass-through load gate. Default: 0
  --max-dlq-depth N          Pass-through load gate. Default: 0
  --feed-timeout-seconds N   Pass-through feed catch-up timeout. Default: 30
  -h, --help                 Show this help

Environment:
  LOAD_SWEEP_PROFILES          Default for --profiles.
  LOAD_SWEEP_REQUIRED_PROFILE  Default for --required-profile.
  LOAD_MAX_P95_SECONDS         Default for --max-p95-seconds.
  LOAD_MAX_P99_SECONDS         Default for --max-p99-seconds.
  LOAD_MIN_SUCCESS_RATE        Default for --min-success-rate.
  LOAD_MAX_5XX                 Default for --max-5xx.
  LOAD_MAX_OUTBOX_DEAD         Default for --max-outbox-dead.
  LOAD_MAX_DLQ_DEPTH           Default for --max-dlq-depth.
  LOAD_FEED_TIMEOUT_SECONDS    Default for --feed-timeout-seconds.

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
    --profiles)
      [[ $# -ge 2 ]] || die "--profiles requires a value"
      profiles="$2"
      shift 2
      ;;
    --required-profile)
      [[ $# -ge 2 ]] || die "--required-profile requires a value"
      required_profile="$2"
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
    --feed-timeout-seconds)
      [[ $# -ge 2 ]] || die "--feed-timeout-seconds requires a value"
      feed_timeout_seconds="$2"
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

[[ -n "$profiles" ]] || die "--profiles must not be empty"
[[ "$frontend_port" =~ ^[0-9]+$ ]] || die "--frontend-port must be an integer"
[[ "$max_5xx" =~ ^[0-9]+$ ]] || die "--max-5xx must be an integer"
[[ "$max_outbox_dead" =~ ^[0-9]+$ ]] || die "--max-outbox-dead must be an integer"
[[ "$max_dlq_depth" =~ ^[0-9]+$ ]] || die "--max-dlq-depth must be an integer"
[[ "$feed_timeout_seconds" =~ ^[0-9]+$ ]] || die "--feed-timeout-seconds must be an integer"
[[ "$max_p95_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--max-p95-seconds must be a non-negative number"
[[ "$max_p99_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--max-p99-seconds must be a non-negative number"
[[ "$min_success_rate" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--min-success-rate must be a non-negative number"
awk -v n="$min_success_rate" 'BEGIN { exit !(n >= 0 && n <= 100) }' || die "--min-success-rate must be between 0 and 100"
(( frontend_port > 0 )) || die "--frontend-port must be greater than zero"
(( feed_timeout_seconds > 0 )) || die "--feed-timeout-seconds must be greater than zero"

cd "$REPO_ROOT"
require_github_actions_evidence_runner "scripts/load-sweep-local.sh"
setup_go_cache

run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
run_dir="$REPO_ROOT/tmp/load-sweep-local/$run_id"
mkdir -p "$run_dir"
results_file="$run_dir/profiles.tsv"
summary_file="$run_dir/summary.md"

printf 'profile\tusers\tcapacity\tconcurrency\texit_code\tgate_failures\tstatus\tthroughput_rps\tp95_seconds\tp99_seconds\tsuccess_rate_percent\terror_rate_percent\tfeed_catchup_seconds\toutbox_dead_rows\tdlq_depth\tresult_dir\n' >"$results_file"

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

wait_for_log_contains() {
  local file="$1"
  local pattern="$2"
  local timeout_seconds="${3:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if [[ -f "$file" ]] && grep -Fq "$pattern" "$file"; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

metric_value() {
  local file="$1"
  local key="$2"
  awk -F '\t' -v key="$key" '$1 == key { print $2; found = 1; exit } END { if (!found) exit 2 }' "$file"
}

is_profile_name() {
  [[ "$1" =~ ^[A-Za-z0-9._-]+$ ]]
}

is_integer() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

numeric_gt() {
  awk -v actual="$1" -v limit="$2" 'BEGIN { exit !(actual + 0 > limit + 0) }'
}

stop_started_stack() {
  "$REPO_ROOT/scripts/stop-local.sh" --frontend-port "$frontend_port" --with-infrastructure >/dev/null 2>&1 || true
  if [[ -n "${stack_pid:-}" ]]; then
    if kill -0 "$stack_pid" >/dev/null 2>&1; then
      kill -INT "$stack_pid" >/dev/null 2>&1 || true
      sleep 1
    fi
    if kill -0 "$stack_pid" >/dev/null 2>&1; then
      kill -TERM "$stack_pid" >/dev/null 2>&1 || true
    fi
    wait "$stack_pid" >/dev/null 2>&1 || true
  fi
}

log "Start local stack for load sweep"
"$REPO_ROOT/scripts/start-local.sh" --frontend-port "$frontend_port" >"$run_dir/start-local.log" 2>&1 &
stack_pid=$!
trap 'stop_started_stack' EXIT
if ! wait_for_url "$base_url/readyz" 120; then
  tail -n 120 "$run_dir/start-local.log" >&2 || true
  die "local stack did not become ready for load sweep."
fi
for url in \
  "http://127.0.0.1:8081/readyz" \
  "http://127.0.0.1:8082/readyz" \
  "http://127.0.0.1:8083/readyz" \
  "http://127.0.0.1:8084/readyz" \
  "http://127.0.0.1:8085/readyz"; do
  if ! wait_for_url "$url" 60; then
    tail -n 120 "$run_dir/start-local.log" >&2 || true
    die "local service did not become ready for load sweep: $url"
  fi
done
if ! wait_for_log_contains "$run_dir/start-local.log" "CityEvents local stack is running." 120; then
  tail -n 120 "$run_dir/start-local.log" >&2 || true
  die "local stack startup did not reach the running sentinel for load sweep."
fi

highest_passing_load_profile="none"
highest_passing_load_concurrency="0"
max_rps_profile="none"
max_rps="0"
max_rps_concurrency="0"
first_failed_profile="none"
first_failed_reason="none"
passing_count=0
required_profile_passed=false

IFS=',' read -ra profile_specs <<<"$profiles"
for spec in "${profile_specs[@]}"; do
  IFS=':' read -r profile users capacity concurrency extra <<<"$spec"
  [[ -z "${extra:-}" ]] || die "invalid profile spec: $spec"
  is_profile_name "${profile:-}" || die "invalid profile name in spec: $spec"
  is_integer "${users:-}" || die "invalid users in spec: $spec"
  is_integer "${capacity:-}" || die "invalid capacity in spec: $spec"
  is_integer "${concurrency:-}" || die "invalid concurrency in spec: $spec"
  (( users > 0 && capacity > 0 && concurrency > 0 )) || die "profile values must be greater than zero: $spec"

  log "Run load sweep profile $profile users=$users capacity=$capacity concurrency=$concurrency"
  profile_log="$run_dir/$profile.log"
  set +e
  bash "$REPO_ROOT/scripts/load-test-local.sh" \
    --base-url "$base_url" \
    --run-label "sweep-$profile" \
    --users "$users" \
    --capacity "$capacity" \
    --concurrency "$concurrency" \
    --min-success-rate "$min_success_rate" \
    --max-p95-seconds "$max_p95_seconds" \
    --max-p99-seconds "$max_p99_seconds" \
    --max-5xx "$max_5xx" \
    --max-outbox-dead "$max_outbox_dead" \
    --max-dlq-depth "$max_dlq_depth" \
    --feed-timeout-seconds "$feed_timeout_seconds" \
    >"$profile_log" 2>&1
  exit_code=$?
  set -e

  result_dir="$(awk -F ': ' '/^- Result files: / { value = $2 } END { print value }' "$profile_log" | tr -d '\r')"
  metrics_file=""
  if [[ -n "$result_dir" ]]; then
    metrics_file="$result_dir/metrics.tsv"
  fi

  if [[ -f "$metrics_file" ]]; then
    gate_failures="$(metric_value "$metrics_file" "gate_failures" || echo "unavailable")"
    throughput="$(metric_value "$metrics_file" "join_throughput_rps" || echo "0")"
    p95="$(metric_value "$metrics_file" "join_latency_p95_seconds" || echo "unavailable")"
    p99="$(metric_value "$metrics_file" "join_latency_p99_seconds" || echo "unavailable")"
    success_rate="$(metric_value "$metrics_file" "join_success_rate_percent" || echo "unavailable")"
    error_rate="$(metric_value "$metrics_file" "join_error_rate_percent" || echo "unavailable")"
    feed_catchup="$(metric_value "$metrics_file" "feed_projection_catchup_seconds" || echo "unavailable")"
    outbox_dead="$(metric_value "$metrics_file" "outbox_dead_rows_after_joins" || echo "unavailable")"
    dlq_depth="$(metric_value "$metrics_file" "rabbitmq_dlq_depth_after_joins" || echo "unavailable")"
  else
    gate_failures="unavailable"
    throughput="0"
    p95="unavailable"
    p99="unavailable"
    success_rate="unavailable"
    error_rate="unavailable"
    feed_catchup="unavailable"
    outbox_dead="unavailable"
    dlq_depth="unavailable"
    result_dir="unavailable"
  fi

  status="failed"
  if [[ "$exit_code" == "0" && "$gate_failures" == "0" ]]; then
    status="passed"
    passing_count=$((passing_count + 1))
    if [[ "$profile" == "$required_profile" ]]; then
      required_profile_passed=true
    fi
    if (( concurrency > highest_passing_load_concurrency )); then
      highest_passing_load_profile="$profile"
      highest_passing_load_concurrency="$concurrency"
    fi
    if numeric_gt "$throughput" "$max_rps"; then
      max_rps_profile="$profile"
      max_rps="$throughput"
      max_rps_concurrency="$concurrency"
    fi
  elif [[ "$first_failed_profile" == "none" ]]; then
    first_failed_profile="$profile"
    first_failed_reason="exit_code=$exit_code gate_failures=$gate_failures"
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$profile" "$users" "$capacity" "$concurrency" "$exit_code" "$gate_failures" "$status" "$throughput" \
    "$p95" "$p99" "$success_rate" "$error_rate" "$feed_catchup" "$outbox_dead" "$dlq_depth" "$result_dir" \
    >>"$results_file"
done

saturation_point="$first_failed_profile"
if [[ "$saturation_point" == "none" ]]; then
  saturation_point="not reached within tested profiles"
fi

sweep_failures=0
if (( passing_count == 0 )); then
  sweep_failures=$((sweep_failures + 1))
fi
if [[ -n "$required_profile" && "$required_profile_passed" != true ]]; then
  sweep_failures=$((sweep_failures + 1))
fi

{
  cat <<EOF
# CI Load Sweep Evidence Summary

- Run ID: $run_id
- Gateway: $base_url
- Profiles: $profiles
- Required passing profile: ${required_profile:-none}
- Passing profiles: $passing_count
- Highest passing load profile: $highest_passing_load_profile
- Highest passing load concurrency: $highest_passing_load_concurrency
- Max stable RPS profile: $max_rps_profile
- Max stable RPS profile concurrency: $max_rps_concurrency
- Max stable RPS candidate: $max_rps
- Saturation point: $saturation_point
- First failed profile detail: $first_failed_reason
- Sweep failures: $sweep_failures
- Profile TSV: $results_file
- Result files: $run_dir

## Interpretation

Highest passing load profile is the largest configured concurrency whose
load-test gates passed. Max stable RPS candidate is the highest observed
throughput among profiles where every load-test gate passed. Both are CI
evidence numbers, not production capacity guarantees.

Saturation point is the first configured profile whose load-test gates failed.
If it says "not reached within tested profiles", the tested range was not high
enough to identify the bottleneck.

## Profile Results

| Profile | Users | Capacity | Concurrency | Status | Exit | Gate Failures | RPS | p95 | p99 | Success % | Error % | Feed Catch-up | Outbox DEAD | DLQ Depth |
| --- | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
EOF
  tail -n +2 "$results_file" | awk -F '\t' '{ printf "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", $1, $2, $3, $4, $7, $5, $6, $8, $9, $10, $11, $12, $13, $14, $15 }'
} >"$summary_file"

cat "$summary_file"
echo
if (( sweep_failures > 0 )); then
  die "CI load sweep evidence failed with $sweep_failures sweep failure(s). See $summary_file"
fi
echo "CI load sweep evidence passed."
