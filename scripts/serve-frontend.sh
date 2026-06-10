#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

port="${1:-18088}"
cd "$REPO_ROOT"
run_node frontend/server.mjs "$port"
