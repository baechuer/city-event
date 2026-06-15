#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

port="${1:-18088}"
cd "$REPO_ROOT"
export CITYEVENTS_API_BASE="${CITYEVENTS_API_BASE:-http://127.0.0.1:8080}"
if [[ ! -d "$REPO_ROOT/frontend/node_modules/react" || ! -d "$REPO_ROOT/frontend/node_modules/vite" ]]; then
  (
    cd "$REPO_ROOT/frontend"
    run_npm ci
  )
fi
(
  cd "$REPO_ROOT/frontend"
  run_npm run build
)
run_node frontend/server.mjs "$port"
