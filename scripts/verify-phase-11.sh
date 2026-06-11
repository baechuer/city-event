#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"

log "Phase 10 baseline verification"
"$REPO_ROOT/scripts/verify-phase-10.sh"

log "Phase 11 HA decision checks"
require_file docs/architecture/high-availability-decision.md
require_file README.md
require_file deploy/kubernetes/README.md
require_file deploy/kubernetes/deployments.yaml

require_contains docs/architecture/high-availability-decision.md "not claiming high availability"
require_contains docs/architecture/high-availability-decision.md "Kubernetes-ready microservices"
require_contains docs/architecture/high-availability-decision.md "replicated stateless workloads"
require_contains docs/architecture/high-availability-decision.md "RabbitMQ quorum queues"
require_contains docs/architecture/high-availability-decision.md "managed or replicated Postgres"
require_contains docs/architecture/high-availability-decision.md "Required Failure Tests Before Claiming HA"
require_contains docs/architecture/high-availability-decision.md "Do not use yet"

require_contains README.md "Phase 11"
require_contains README.md "They do not prove high availability"
require_contains README.md "Verify High Availability Decision"
require_contains README.md "Not yet allowed"
require_contains README.md "highly available Kubernetes deployment"

require_contains deploy/kubernetes/README.md "do not prove high availability"
require_contains deploy/kubernetes/README.md "high-availability-decision.md"

[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "replicas: 2")" -eq 10 ]] || die "Phase 11 expects ten replicated readiness Deployments."
require_not_contains deploy/kubernetes/deployments.yaml "kind: HorizontalPodAutoscaler"
[[ "$(count_occurrences deploy/kubernetes/poddisruptionbudgets.yaml "kind: PodDisruptionBudget")" -eq 10 ]] || die "Phase 11 expects ten PodDisruptionBudget resources."

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 11 verification completed."
