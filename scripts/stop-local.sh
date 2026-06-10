#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

stop_infrastructure=false
remove_volumes=false
frontend_port=18088

usage() {
  cat <<'EOF'
Usage: ./scripts/stop-local.sh [options]

Stops local CityEvents processes started by scripts/start-local.sh.

Options:
  --frontend-port PORT  Frontend port to clean stale local servers for. Default: 18088
  --with-infrastructure  Also run docker compose down
  --volumes              Also remove Docker Compose volumes; implies --with-infrastructure
  -h, --help             Show this help

Default behavior stops the Go services, async workers, and frontend only.
Docker containers and local data are left running unless explicitly requested.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --frontend-port)
      [[ $# -ge 2 ]] || die "--frontend-port requires a value"
      frontend_port="$2"
      shift 2
      ;;
    --with-infrastructure)
      stop_infrastructure=true
      shift
      ;;
    --volumes)
      stop_infrastructure=true
      remove_volumes=true
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

run_dir="$REPO_ROOT/tmp/local-run"
pid_file="$run_dir/pids.tsv"
launcher_pid_file="$run_dir/launcher.pid"

service_names=(
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

stop_pid() {
  local pid="${1:-}"
  local name="${2:-process}"
  [[ -n "$pid" ]] || return 0
  if kill -0 "$pid" >/dev/null 2>&1; then
    echo "Stopping $name pid $pid"
    kill "$pid" >/dev/null 2>&1 || true
    local deadline=$((SECONDS + 5))
    while (( SECONDS < deadline )); do
      kill -0 "$pid" >/dev/null 2>&1 || return 0
      sleep 0.2
    done
    kill -9 "$pid" >/dev/null 2>&1 || true
  fi
}

stop_windows_service_image() {
  local name="$1"
  if command -v taskkill.exe >/dev/null 2>&1; then
    taskkill.exe /F /T /IM "$name.exe" >/dev/null 2>&1 || true
  elif command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "Get-Process -Name '$name' -ErrorAction SilentlyContinue | Stop-Process -Force" >/dev/null 2>&1 || true
  fi
}

stop_local_binary_by_path() {
  local name="$1"
  if command -v pkill >/dev/null 2>&1; then
    pkill -f "$REPO_ROOT/tmp/local-bin/$name.exe" >/dev/null 2>&1 || true
  fi
}

if [[ -f "$launcher_pid_file" ]]; then
  launcher_pid="$(tr -d '[:space:]' <"$launcher_pid_file")"
  stop_pid "$launcher_pid" "launcher"
  sleep 1
fi

if [[ -f "$pid_file" ]]; then
  mapfile -t tracked <"$pid_file"
  for (( i=${#tracked[@]} - 1; i >= 0; i-- )); do
    [[ -n "${tracked[$i]}" ]] || continue
    IFS=$'\t' read -r name pid _ <<<"${tracked[$i]}"
    stop_pid "$pid" "$name"
  done
  rm -f "$pid_file"
else
  echo "No service pid file found at $pid_file"
fi

for service in "${service_names[@]}"; do
  stop_windows_service_image "$service"
  stop_local_binary_by_path "$service"
done

stop_frontend_servers_on_port "$frontend_port"

if [[ "$stop_infrastructure" == true ]]; then
  if [[ "$remove_volumes" == true ]]; then
    log "Stop Docker Compose infrastructure and remove volumes"
    run_docker compose down -v
  else
    log "Stop Docker Compose infrastructure"
    run_docker compose down
  fi
else
  echo "Docker dependencies were left as-is. Use --with-infrastructure to run docker compose down."
fi

echo "Local CityEvents termination completed."
