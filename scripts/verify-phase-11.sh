#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"

log "Phase 10 baseline verification"
"$REPO_ROOT/scripts/verify-phase-10.sh"

log "Phase 11 HA decision checks"
require_file README.md
require_file deploy/kubernetes/README.md
require_file deploy/kubernetes/deployments.yaml

require_contains README.md "Phase 11"
require_contains README.md "They do not prove high availability"
require_contains README.md "Verify High Availability Decision"
require_contains README.md "Not yet allowed"
require_contains README.md "highly available Kubernetes deployment"

require_contains deploy/kubernetes/README.md "do not prove high availability"
require_contains deploy/kubernetes/README.md "High availability still requires"

[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "replicas: 2")" -eq 10 ]] || die "Phase 11 expects ten replicated readiness Deployments."
require_not_contains deploy/kubernetes/deployments.yaml "kind: HorizontalPodAutoscaler"
[[ "$(count_occurrences deploy/kubernetes/poddisruptionbudgets.yaml "kind: PodDisruptionBudget")" -eq 10 ]] || die "Phase 11 expects ten PodDisruptionBudget resources."

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 11 verification completed."
