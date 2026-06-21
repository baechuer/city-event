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
  if command -v docker >/dev/null 2>&1 && docker version >/dev/null 2>&1; then
    docker "$@"
    return
  fi
  if command -v docker.exe >/dev/null 2>&1 && docker.exe version >/dev/null 2>&1; then
    docker.exe "$@"
    return
  fi
  if command -v cmd.exe >/dev/null 2>&1; then
    # Intentionally unquoted for the same reason as run_go: current call sites
    # pass simple Docker arguments and cmd.exe receives them reliably this way.
    cmd.exe /c docker $*
    return
  fi
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

valid_node_candidate() {
  local candidate="$1"
  [[ -n "$candidate" ]] || return 1
  "$candidate" -v >/dev/null 2>&1
}

node_bin() {
  local candidate
  for candidate in \
    "$(command -v node 2>/dev/null || true)" \
    "$(command -v node.exe 2>/dev/null || true)" \
    "/mnt/c/nvm4w/nodejs/node.exe" \
    "/c/nvm4w/nodejs/node.exe" \
    "/mnt/c/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe" \
    "/c/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe"; do
    if valid_node_candidate "$candidate"; then
      echo "$candidate"
      return
    fi
  done
  die "node was not found. Install Node or use the bundled Codex runtime."
}

run_node() {
  local node
  node="$(node_bin)"
  "$node" "$@"
}

run_npm() {
  if command -v npm >/dev/null 2>&1; then
    npm "$@"
    return
  fi
  if command -v npm.cmd >/dev/null 2>&1; then
    npm.cmd "$@"
    return
  fi
  if command -v cmd.exe >/dev/null 2>&1 && cmd.exe /c "where npm" >/dev/null 2>&1; then
    cmd.exe /c npm $*
    return
  fi
  die "npm was not found. Install Node/npm or use the bundled Codex runtime."
}

http_curl() {
  if command -v curl.exe >/dev/null 2>&1; then
    curl.exe "$@"
    return
  fi
  curl "$@"
}

http_null_target() {
  if command -v curl.exe >/dev/null 2>&1; then
    echo "NUL"
    return
  fi
  echo "/dev/null"
}

windows_binary_path() {
  local path="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$path"
    return
  fi
  if command -v wslpath >/dev/null 2>&1; then
    wslpath -w "$path"
    return
  fi
  echo "$path"
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
    if http_curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
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

jwt_for_user() {
  local user_id="$1"
  local role="${2:-USER}"
  local secret="${JWT_SECRET:-dev-secret-change-me}"
  local issuer="${JWT_ISSUER:-cityevents}"
  run_node - "$user_id" "$role" "$secret" "$issuer" <<'NODE'
const crypto = require('node:crypto');

const [userID, role, secret, issuer] = process.argv.slice(2);
const now = Math.floor(Date.now() / 1000);
const base64url = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
const header = base64url({ alg: 'HS256', typ: 'JWT' });
const payload = base64url({
  sub: userID,
  email: `${userID}@example.com`,
  role,
  jti: crypto.randomUUID(),
  iss: issuer,
  iat: now,
  exp: now + 3600,
});
const unsigned = `${header}.${payload}`;
const signature = crypto.createHmac('sha256', secret).update(unsigned).digest('base64url');
process.stdout.write(`${unsigned}.${signature}`);
NODE
}

cleanup_pid() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
}

require_github_actions_evidence_runner() {
  local script_name="$1"
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    return 0
  fi

  cat >&2 <<EOF
error: $script_name is a heavy evidence script and is approved only on GitHub Actions.

Local workstation execution is blocked to protect Docker/WSL/host stability.
Run the manual GitHub Actions heavy-evidence workflow instead.
EOF
  exit 1
}

stop_frontend_servers_on_port() {
  local frontend_port="$1"
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "\$port = '$frontend_port'; Get-CimInstance Win32_Process -Filter \"Name = 'node.exe' OR Name = 'python.exe' OR Name = 'python3.exe'\" | Where-Object { \$_.CommandLine -and ((\$_.CommandLine -match 'frontend[/\\\\]server\.mjs' -and \$_.CommandLine -match ('\\s' + \$port + '(\\s|\$)')) -or (\$_.CommandLine -match 'http\.server' -and \$_.CommandLine -match ('\\s' + \$port + '(\\s|\$)'))) } | ForEach-Object { Write-Output ('Stopping stale frontend server pid ' + \$_.ProcessId); Stop-Process -Id \$_.ProcessId -Force }" || true
  fi
}
