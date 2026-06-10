#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

frontend_port=18088
skip_build=false
startup_check=false

usage() {
  cat <<'EOF'
Usage: ./scripts/start-local.sh [options]

Starts the local CityEvents demo stack:
  - Docker Compose dependencies: Postgres, RabbitMQ, Redis, MinIO, Mailpit
  - Go HTTP services: gateway, auth, event registration, feed, notification, media
  - Go async workers: outbox relay, feed worker, notification worker, media worker
  - Static frontend at http://127.0.0.1:18088

Options:
  --frontend-port PORT  Frontend port to bind. Default: 18088
  --skip-build          Reuse binaries from tmp/local-bin
  --check               Start everything, verify readiness, then stop local processes
  -h, --help            Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --frontend-port)
      [[ $# -ge 2 ]] || die "--frontend-port requires a value"
      frontend_port="$2"
      shift 2
      ;;
    --skip-build)
      skip_build=true
      shift
      ;;
    --check)
      startup_check=true
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
setup_go_cache

bin_dir="$REPO_ROOT/tmp/local-bin"
logs_dir="$REPO_ROOT/tmp/local-logs"
run_dir="$REPO_ROOT/tmp/local-run"
pid_file="$run_dir/pids.tsv"
mkdir -p "$bin_dir" "$logs_dir" "$run_dir"
: >"$pid_file"

postgres_url="postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
rabbitmq_url="amqp://cityevents:cityevents@localhost:5672/"

common_env=(
  "CITYEVENTS_ENV=local"
  "POSTGRES_URL=$postgres_url"
  "RABBITMQ_URL=$rabbitmq_url"
  "REDIS_URL=redis://localhost:6379/0"
  "MINIO_ENDPOINT=http://localhost:9000"
  "MINIO_ACCESS_KEY=cityevents"
  "MINIO_SECRET_KEY=cityevents-password"
  "MINIO_BUCKET=cityevents-media"
  "SMTP_ADDR=localhost:1025"
  "JWT_SECRET=local-dev-secret-not-for-production"
  "JWT_ISSUER=cityevents-local"
)

http_services=(
  "api-gateway:API_GATEWAY_HTTP_ADDR:8080"
  "auth-service:AUTH_SERVICE_HTTP_ADDR:8081"
  "event-registration-service:EVENT_REGISTRATION_SERVICE_HTTP_ADDR:8082"
  "feed-service:FEED_SERVICE_HTTP_ADDR:8083"
  "notification-service:NOTIFICATION_SERVICE_HTTP_ADDR:8084"
  "media-service:MEDIA_SERVICE_HTTP_ADDR:8085"
)

worker_services=(
  "outbox-relay"
  "feed-worker"
  "notification-worker"
  "media-worker"
)

all_services=(
  "api-gateway"
  "auth-service"
  "event-registration-service"
  "feed-service"
  "notification-service"
  "media-service"
  "outbox-relay"
  "feed-worker"
  "notification-worker"
  "media-worker"
)

pids=()
names=()
log_files=()

use_windows_process_launcher() {
  command -v wslpath >/dev/null 2>&1 &&
    ! command -v go >/dev/null 2>&1 &&
    command -v cmd.exe >/dev/null 2>&1
}

wait_for_backend_http() {
  local url="$1"
  local timeout_seconds="${2:-30}"
  local deadline=$((SECONDS + timeout_seconds))

  if use_windows_process_launcher && command -v powershell.exe >/dev/null 2>&1; then
    while (( SECONDS < deadline )); do
      if powershell.exe -NoProfile -Command "try { \$r = Invoke-WebRequest -UseBasicParsing -Uri '$url' -TimeoutSec 2; if (\$r.StatusCode -ge 200 -and \$r.StatusCode -lt 400) { exit 0 }; exit 1 } catch { exit 1 }" >/dev/null 2>&1; then
        return 0
      fi
      sleep 0.3
    done
    return 1
  fi

  wait_for_http "$url" "$timeout_seconds"
}

show_log_tail() {
  local name="$1"
  local file="$2"
  echo "---- $name log: $file ----" >&2
  tail -n 80 "$file" >&2 || true
}

cleanup() {
  local i
  for (( i=${#pids[@]} - 1; i >= 0; i-- )); do
    if use_windows_process_launcher && [[ "${names[$i]}" != "frontend" ]]; then
      taskkill.exe /F /T /IM "${names[$i]}.exe" >/dev/null 2>&1 || true
    fi
    cleanup_pid "${pids[$i]}"
  done
  rm -f "$pid_file"
}

shutdown() {
  trap - EXIT INT TERM
  cleanup
  exit 0
}

trap cleanup EXIT
trap shutdown INT TERM

record_pid() {
  local name="$1"
  local pid="$2"
  local log_file="$3"
  pids+=("$pid")
  names+=("$name")
  log_files+=("$log_file")
  printf '%s\t%s\t%s\n' "$name" "$pid" "$log_file" >>"$pid_file"
}

assert_process_running() {
  local index="$1"
  local pid="${pids[$index]}"
  local name="${names[$index]}"
  local log_file="${log_files[$index]}"
  if ! kill -0 "$pid" >/dev/null 2>&1; then
    show_log_tail "$name" "$log_file"
    die "$name exited unexpectedly."
  fi
}

check_all_processes() {
  local i
  for (( i=0; i<${#pids[@]}; i++ )); do
    assert_process_running "$i"
  done
}

build_service() {
  local name="$1"
  log "Build $name"
  run_go build -o "tmp/local-bin/$name.exe" "./cmd/$name"
  chmod +x "$bin_dir/$name.exe" 2>/dev/null || true
}

start_binary() {
  local name="$1"
  shift
  local binary="$bin_dir/$name.exe"
  local log_file="$logs_dir/$name.log"

  [[ -f "$binary" ]] || die "missing binary $binary. Rerun without --skip-build."
  : >"$log_file"

  log "Start $name"
  if use_windows_process_launcher; then
    local win_binary
    local cmd_line
    local kv
    win_binary="$(wslpath -w "$binary")"
    cmd_line=""
    for kv in "${common_env[@]}" "$@"; do
      # WSL interop does not reliably preserve cmd.exe's set "KEY=value"
      # quoting form, so keep these local-dev values simple and unquoted.
      cmd_line="${cmd_line}set $kv && "
    done
    cmd_line="${cmd_line}$win_binary"
    cmd.exe /d /c "$cmd_line" >"$log_file" 2>&1 &
  else
    (
      cd "$REPO_ROOT"
      env "${common_env[@]}" "$@" "$binary"
    ) >"$log_file" 2>&1 &
  fi
  local pid=$!
  record_pid "$name" "$pid" "$log_file"
  sleep 0.4
  assert_process_running "$((${#pids[@]} - 1))"
}

start_frontend() {
  local log_file="$logs_dir/frontend.log"
  : >"$log_file"

  log "Start frontend"
  if command -v python3 >/dev/null 2>&1; then
    (
      cd "$REPO_ROOT/frontend"
      python3 -m http.server "$frontend_port" --bind 127.0.0.1
    ) >"$log_file" 2>&1 &
  elif command -v python >/dev/null 2>&1; then
    (
      cd "$REPO_ROOT/frontend"
      python -m http.server "$frontend_port" --bind 127.0.0.1
    ) >"$log_file" 2>&1 &
  else
    (
      cd "$REPO_ROOT"
      run_node frontend/server.mjs "$frontend_port"
    ) >"$log_file" 2>&1 &
  fi
  local pid=$!
  record_pid "frontend" "$pid" "$log_file"
  sleep 0.4
  assert_process_running "$((${#pids[@]} - 1))"
}

log "Start local infrastructure"
run_docker compose up -d postgres rabbitmq redis minio mailpit
wait_for_compose_health postgres 120
wait_for_compose_health rabbitmq 120
wait_for_compose_health redis 120
wait_for_compose_health minio 120
wait_for_tcp 127.0.0.1 1025 60

if [[ "$skip_build" == false ]]; then
  for service in "${all_services[@]}"; do
    build_service "$service"
  done
fi

for spec in "${http_services[@]}"; do
  IFS=":" read -r service env_key port <<<"$spec"
  start_binary "$service" "$env_key=:$port"
done

for spec in "${http_services[@]}"; do
  IFS=":" read -r service _ port <<<"$spec"
  if ! wait_for_backend_http "http://127.0.0.1:$port/readyz" 30; then
    show_log_tail "$service" "$logs_dir/$service.log"
    die "$service did not become ready on port $port."
  fi
done

for service in "${worker_services[@]}"; do
  start_binary "$service"
done

start_frontend
if ! wait_for_http "http://127.0.0.1:$frontend_port/" 20; then
  show_log_tail "frontend" "$logs_dir/frontend.log"
  die "frontend did not become ready on port $frontend_port."
fi

sleep 1
check_all_processes

cat <<EOF

CityEvents local stack is running.

Entry point:
  http://127.0.0.1:$frontend_port

Backend health:
  api-gateway                 http://127.0.0.1:8080/readyz
  auth-service                http://127.0.0.1:8081/readyz
  event-registration-service  http://127.0.0.1:8082/readyz
  feed-service                http://127.0.0.1:8083/readyz
  notification-service        http://127.0.0.1:8084/readyz
  media-service               http://127.0.0.1:8085/readyz

Local consoles:
  RabbitMQ  http://127.0.0.1:15672
  MinIO     http://127.0.0.1:9001
  Mailpit   http://127.0.0.1:8025

Logs:
  $logs_dir

Press Ctrl-C to stop the Go services and frontend.
From another terminal, stop the local app with: ./scripts/stop-local.sh
Docker dependencies are left running; stop them too with: ./scripts/stop-local.sh --with-infrastructure
EOF

if [[ "$startup_check" == true ]]; then
  log "Startup check completed"
  exit 0
fi

while true; do
  sleep 2
  check_all_processes
done
