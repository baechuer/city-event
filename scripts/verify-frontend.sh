#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT/frontend"

if [[ ! -d node_modules/typescript || ! -d node_modules/tsx || ! -d node_modules/vite ]]; then
  if command -v npm >/dev/null 2>&1 || command -v npm.cmd >/dev/null 2>&1; then
    run_npm ci
  else
    die "frontend node_modules are missing and npm is unavailable. Run npm ci once, or set CITYEVENTS_NODE to a Node runtime when node_modules already exist."
  fi
fi

run_npm run verify
