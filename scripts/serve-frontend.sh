#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

port="${1:-18088}"
cd "$REPO_ROOT"
export CITYEVENTS_API_BASE="${CITYEVENTS_API_BASE:-http://127.0.0.1:8080}"
run_node frontend/server.mjs "$port"
