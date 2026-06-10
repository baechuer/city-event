#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

die() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "== $* =="
}

setup_go_cache() {
  mkdir -p "$REPO_ROOT/.cache/go-build"
  if command -v go >/dev/null 2>&1; then
    export GOCACHE="$REPO_ROOT/.cache/go-build"
  elif command -v wslpath >/dev/null 2>&1; then
    export GOCACHE="$(wslpath -w "$REPO_ROOT/.cache/go-build")"
  else
    export GOCACHE="$REPO_ROOT/.cache/go-build"
  fi
}

run_go() {
  if command -v go >/dev/null 2>&1; then
    go "$@"
    return
  fi
  if command -v cmd.exe >/dev/null 2>&1; then
    # Intentionally unquoted: cmd.exe /c only receives the full command reliably
    # this way from WSL. Current script call sites pass simple arguments only.
    cmd.exe /c go $*
    return
  fi
  die "go was not found. Install Go or run from a shell that can access go.exe."
}

run_docker() {
  docker "$@"
}

run_git() {
  local windows_git="/mnt/c/Program Files/Git/cmd/git.exe"
  if [[ -x "$windows_git" ]]; then
    "$windows_git" "$@"
    return
  fi
  if command -v git.exe >/dev/null 2>&1; then
    git.exe "$@"
    return
  fi
  git "$@"
}

node_bin() {
  if command -v node >/dev/null 2>&1; then
    command -v node
    return
  fi
  local bundled="/mnt/c/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe"
  if [[ -x "$bundled" ]]; then
    echo "$bundled"
    return
  fi
  die "node was not found. Install Node or use the bundled Codex runtime."
}

run_node() {
  local node
  node="$(node_bin)"
  "$node" "$@"
}

require_file() {
  [[ -e "$1" ]] || die "required file is missing: $1"
}

require_contains() {
  local file="$1"
  local needle="$2"
  grep -Fq "$needle" "$file" || die "$file is missing required content: $needle"
}

require_not_contains() {
  local file="$1"
  local needle="$2"
  if grep -Fq "$needle" "$file"; then
    die "$file contains forbidden content: $needle"
  fi
}

count_occurrences() {
  local file="$1"
  local needle="$2"
  grep -Fo "$needle" "$file" | wc -l | tr -d ' '
}

wait_for_compose_health() {
  local service="$1"
  local timeout_seconds="${2:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  local status=""
  while (( SECONDS < deadline )); do
    local cid
    cid="$(run_docker compose ps -q "$service" 2>/dev/null || true)"
    if [[ -n "$cid" ]]; then
      status="$(run_docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null || true)"
      if [[ "$status" == "healthy" || "$status" == "running" ]]; then
        if [[ "$status" == "running" ]]; then
          local has_health
          has_health="$(run_docker inspect -f '{{if .State.Health}}yes{{end}}' "$cid" 2>/dev/null || true)"
          [[ "$has_health" == "yes" ]] || return 0
        else
          return 0
        fi
      fi
    fi
    sleep 3
  done
  run_docker compose ps "$service" || true
  die "$service did not become healthy."
}

wait_for_tcp() {
  local host="$1"
  local port="$2"
  local timeout_seconds="${3:-60}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if timeout 2 bash -c ":</dev/tcp/$host/$port" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  die "$host:$port did not become reachable."
}

wait_for_http() {
  local url="$1"
  local timeout_seconds="${2:-20}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.3
  done
  return 1
}

json_string_field() {
  local json="$1"
  local field="$2"
  printf '%s' "$json" | sed -n "s/.*\"$field\":\"\\([^\"]*\\)\".*/\\1/p"
}

cleanup_pid() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
}
