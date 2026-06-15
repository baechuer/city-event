#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"
setup_go_cache

log "Backend default Go tests"
run_go test ./...

log "Frontend React/TypeScript checks"
(cd frontend && run_npm run verify)

log "Static frontend file checks"
for path in \
  frontend/index.html \
  frontend/server.mjs \
  frontend/vite.config.ts \
  frontend/tsconfig.json \
  frontend/src/App.tsx \
  frontend/src/main.tsx \
  frontend/src/api.ts \
  frontend/src/state.ts \
  frontend/src/styles.css; do
  require_file "$path"
done

echo "Phase 8 verification completed."
