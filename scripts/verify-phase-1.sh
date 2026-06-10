#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

start_infrastructure=false
require_docker_daemon=false
for arg in "$@"; do
  case "$arg" in
    --start-infrastructure) start_infrastructure=true ;;
    --require-docker-daemon) require_docker_daemon=true ;;
    *) die "unknown argument: $arg" ;;
  esac
done

cd "$REPO_ROOT"
setup_go_cache

services=(
  api-gateway
  auth-service
  event-registration-service
  feed-service
  notification-service
  media-service
  media-worker
)

log "Go tests"
run_go test ./...

log "Docker Compose config"
run_docker compose config --quiet

log "Service startup checks"
previous_startup_check="${CITYEVENTS_STARTUP_CHECK_ONLY-}"
export CITYEVENTS_STARTUP_CHECK_ONLY=true
for service in "${services[@]}"; do
  echo "checking $service"
  run_go run "./cmd/$service"
done
if [[ -n "${previous_startup_check}" ]]; then
  export CITYEVENTS_STARTUP_CHECK_ONLY="$previous_startup_check"
else
  unset CITYEVENTS_STARTUP_CHECK_ONLY
fi

log "Docker daemon check"
if docker_output="$(run_docker info --format '{{.ServerVersion}}' 2>&1)"; then
  echo "$docker_output"
  docker_available=true
else
  docker_available=false
  if [[ "$require_docker_daemon" == true ]]; then
    die "$docker_output"
  fi
  echo "warning: Docker daemon is unavailable; skipping live infrastructure start." >&2
fi

if [[ "$start_infrastructure" == true ]]; then
  [[ "$docker_available" == true ]] || die "Cannot start infrastructure because Docker daemon is unavailable."
  log "Starting local infrastructure"
  run_docker compose up -d
  run_docker compose ps
fi

echo "Phase 1 verification completed."
