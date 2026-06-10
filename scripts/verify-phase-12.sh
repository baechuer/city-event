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

log "Final evidence document checks"
require_file docs/architecture/final-evidence-audit.md
require_file docs/architecture/high-availability-decision.md
require_file docs/resume/resume-claims.md
require_file README.md

require_contains docs/architecture/final-evidence-audit.md "Evidence Matrix"
require_contains docs/architecture/final-evidence-audit.md "Exactly-once consumption"
require_contains docs/architecture/final-evidence-audit.md "Not supported"
require_contains docs/architecture/final-evidence-audit.md "at-least-once messaging"
require_contains docs/architecture/final-evidence-audit.md "idempotent business effects"
require_contains docs/architecture/final-evidence-audit.md "The project is not yet a production HA system"

require_contains docs/resume/resume-claims.md "Recommended Resume Bullets"
require_contains docs/resume/resume-claims.md "exactly-once RabbitMQ consumption"
require_contains docs/resume/resume-claims.md "Kubernetes-ready"
require_contains docs/resume/resume-claims.md "Current Limitation Statement"

require_contains README.md "Phase 12"
require_contains README.md "Verify Final Evidence Audit"
require_contains README.md "Phase 12 Claim Boundary"
require_contains README.md "Exactly-once RabbitMQ consumption"
require_contains README.md "guaranteed no message loss"

log "Safe bullet sanity checks"
recommended="$(sed -n '/^## Recommended Resume Bullets/,/^## Short Version/p' docs/resume/resume-claims.md)"
grep -Fq "exactly-once" <<<"$recommended" && die "Recommended Resume Bullets contains exactly-once"
grep -Fq "guaranteed" <<<"$recommended" && die "Recommended Resume Bullets contains guaranteed"
grep -Fq "highly available" <<<"$recommended" && die "Recommended Resume Bullets contains highly available"
grep -Fq "production deployed" <<<"$recommended" && die "Recommended Resume Bullets contains production deployed"

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 12 verification completed."
