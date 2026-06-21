#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

run_full_integration=false
for arg in "$@"; do
  case "$arg" in
    --run-full-integration) run_full_integration=true ;;
    *) die "unknown argument: $arg" ;;
  esac
done

cd "$REPO_ROOT"

log "Phase 11 baseline verification"
"$REPO_ROOT/scripts/verify-phase-11.sh"

if [[ "$run_full_integration" == true ]]; then
  log "Full integration verification"
  "$REPO_ROOT/scripts/verify-phase-9.sh"
fi

log "Public evidence boundary checks"
require_file README.md

require_contains README.md "RabbitMQ And Transactional Outbox"
require_contains README.md "This is an at-least-once messaging design"
require_contains README.md "effectively-once"
require_contains README.md "business effects"
require_contains README.md "broker dedupe is still not the same as end-to-end exactly-once business effects"
require_contains README.md "manifests alone are not the same as a production"

log "Public README safe-claim sanity checks"
require_not_contains README.md "Exactly-once RabbitMQ consumption"
require_not_contains README.md "guaranteed no message loss"
require_not_contains README.md "production deployed"
require_not_contains README.md "fully highly available"
require_not_contains README.md "zero data loss under all failures"

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 12 verification completed."
