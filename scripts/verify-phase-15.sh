#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

run_live=false

usage() {
  cat <<'EOF'
Usage: ./scripts/verify-phase-15.sh [options]

Verifies Phase 15+ hardening rubric and Kubernetes live-evidence tooling.
The default path is static and CI-safe.

Options:
  --run-live   Run Minikube repair and Kubernetes live smoke inside GitHub Actions
  -h, --help   Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --run-live)
      run_live=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

cd "$REPO_ROOT"

log "Phase 14 baseline verification"
"$REPO_ROOT/scripts/verify-phase-14.sh"

log "Phase 15 file coverage"
for file in \
  docs/architecture/phase-15-hardening-rubric.md \
  scripts/repair-minikube.sh \
  scripts/k8s-live-smoke.sh; do
  require_file "$file"
done

require_contains docs/architecture/phase-15-hardening-rubric.md "Phase 15: Kubernetes Live Evidence"
require_contains docs/architecture/phase-15-hardening-rubric.md "Phase 18: Redis Distributed Rate Limiting"
require_contains docs/architecture/phase-15-hardening-rubric.md "p50, p95, p99"
require_contains scripts/repair-minikube.sh "delete_profile=true"
require_contains scripts/repair-minikube.sh "wait_for_cluster_access"
require_contains scripts/k8s-live-smoke.sh "A completed run is local Kubernetes evidence"
require_contains scripts/k8s-live-smoke.sh "dependency_images"
require_contains scripts/k8s-live-smoke.sh "Load dependency image into Minikube"
require_contains scripts/k8s-live-smoke.sh "ensure_local_port_free"
require_contains scripts/k8s-live-smoke.sh "require_github_actions_evidence_runner"
require_contains scripts/repair-minikube.sh "require_github_actions_evidence_runner"
require_contains scripts/load-test-local.sh "require_github_actions_evidence_runner"
require_file docs/testing/heavy-evidence-runner-policy.md
require_contains docs/testing/heavy-evidence-runner-policy.md "Heavy evidence must not run from the local workstation"
require_contains docs/testing/heavy-evidence-runner-policy.md "scripts/repair-minikube.sh"
require_not_contains scripts/k8s-live-smoke.sh "Stop-Process"
require_not_contains scripts/k8s-live-smoke.sh "Get-CimInstance Win32_Process"

log "Bash syntax"
bash -n scripts/*.sh scripts/lib/*.sh

if [[ "$run_live" == true ]]; then
  require_github_actions_evidence_runner "scripts/verify-phase-15.sh --run-live"
  log "Minikube repair"
  "$REPO_ROOT/scripts/repair-minikube.sh" --delete-profile
  log "Kubernetes live smoke"
  "$REPO_ROOT/scripts/k8s-live-smoke.sh" --start-minikube --run-failure
else
  cat <<'EOF'
Live Kubernetes evidence skipped. To repair the profile and run live evidence in GitHub Actions:

  ./scripts/repair-minikube.sh --delete-profile
  ./scripts/k8s-live-smoke.sh --start-minikube --run-failure

This is host-level Minikube evidence. Local workstation execution is blocked by
policy; use the manual heavy-evidence GitHub Actions workflow.
EOF
fi

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 15 verification completed."
