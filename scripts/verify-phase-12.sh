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

require_contains README.md "Verify Final Evidence Audit"
require_contains README.md "Claim Boundary"
require_contains README.md "Exactly-once RabbitMQ consumption"
require_contains README.md "guaranteed no message loss"

log "Public README safe-claim sanity checks"
allowed="$(sed -n '/^Allowed wording:/,/^Not yet allowed:/p' README.md)"
grep -Fq "exactly-once" <<<"$allowed" && die "Allowed README wording contains exactly-once"
grep -Fq "guaranteed" <<<"$allowed" && die "Allowed README wording contains guaranteed"
grep -Fq "highly available" <<<"$allowed" && die "Allowed README wording contains highly available"
grep -Fq "production deployed" <<<"$allowed" && die "Allowed README wording contains production deployed"

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 12 verification completed."
