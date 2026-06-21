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

require_contains README.md "Kubernetes Manifests As Deployment Readiness"
require_contains README.md "manifests alone are not the same as a production"
require_contains README.md "real multi-node cluster"
require_contains README.md "replicas, probes, ConfigMaps, Secret examples, ingress, PDBs, HPA intent"

require_contains deploy/kubernetes/README.md "Hardening Demonstrated"
require_contains deploy/kubernetes/README.md "Ingress plus replicas are an important deployment foundation"
require_contains deploy/kubernetes/README.md "managed backing services"
require_contains deploy/kubernetes/README.md "real multi-node cluster"

[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "replicas: 2")" -eq 10 ]] || die "Phase 11 expects ten replicated readiness Deployments."
require_not_contains deploy/kubernetes/deployments.yaml "kind: HorizontalPodAutoscaler"
[[ "$(count_occurrences deploy/kubernetes/poddisruptionbudgets.yaml "kind: PodDisruptionBudget")" -eq 10 ]] || die "Phase 11 expects ten PodDisruptionBudget resources."

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 11 verification completed."
