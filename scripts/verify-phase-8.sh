#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"
setup_go_cache

log "Backend default Go tests"
run_go test ./...

log "Frontend JavaScript tests"
run_node --test frontend/src/*.test.mjs

log "Static frontend file checks"
for path in \
  frontend/index.html \
  frontend/server.mjs \
  frontend/src/app.js \
  frontend/src/api.js \
  frontend/src/state.js \
  frontend/src/styles.css; do
  require_file "$path"
done

echo "Phase 8 verification completed."
