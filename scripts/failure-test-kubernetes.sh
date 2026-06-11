#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

target_deployment="api-gateway"
live=false

usage() {
  cat <<'EOF'
Usage: ./scripts/failure-test-kubernetes.sh [options]

Validates Kubernetes replica/failure-test readiness. By default this script
performs static manifest checks only. It mutates a cluster only with --live.

Options:
  --live                  Apply manifests and delete one pod to verify replacement
  --deployment NAME       Deployment to delete one pod from during --live test
                          Default: api-gateway
  -h, --help              Show this help

Live test prerequisites:
  - GitHub Actions runner
  - working kubectl context
  - cityevents images available to the cluster
  - Kubernetes dependencies/secrets adjusted for that cluster
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --live)
      live=true
      shift
      ;;
    --deployment)
      [[ $# -ge 2 ]] || die "--deployment requires a value"
      target_deployment="$2"
      shift 2
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

if [[ "$live" == true ]]; then
  require_github_actions_evidence_runner "scripts/failure-test-kubernetes.sh --live"
fi

log "Kubernetes replica and disruption-budget static checks"
require_file deploy/kubernetes/deployments.yaml
require_file deploy/kubernetes/poddisruptionbudgets.yaml
require_file deploy/kubernetes/kustomization.yaml
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "replicas: 2")" -eq 10 ]] || die "expected ten deployments with replicas: 2"
[[ "$(count_occurrences deploy/kubernetes/poddisruptionbudgets.yaml "kind: PodDisruptionBudget")" -eq 10 ]] || die "expected ten PodDisruptionBudget resources"
require_contains deploy/kubernetes/kustomization.yaml "poddisruptionbudgets.yaml"

if command -v kubectl >/dev/null 2>&1; then
  log "kubectl kustomize"
  kubectl kustomize deploy/kubernetes >/dev/null
else
  echo "kubectl not found; skipped kustomize validation and live failure test."
  exit 0
fi

if [[ "$live" != true ]]; then
  cat <<EOF
Static checks passed.

Live pod-failure test is intentionally skipped. To run it against the current
kubectl context:

  ./scripts/failure-test-kubernetes.sh --live --deployment $target_deployment

Only treat HA/failure-recovery claims as proven after the live test runs against
a real cluster with available images and configured dependencies.
EOF
  exit 0
fi

log "Check Kubernetes cluster access"
kubectl cluster-info --request-timeout=5s >/dev/null

log "Apply Kubernetes manifests"
kubectl apply -k deploy/kubernetes

log "Wait for deployments"
kubectl -n cityevents wait --for=condition=available deployment --all --timeout=180s

log "Delete one pod for $target_deployment"
pod_name="$(kubectl -n cityevents get pods \
  -l "app.kubernetes.io/component=$target_deployment" \
  -o jsonpath='{.items[0].metadata.name}')"
[[ -n "$pod_name" ]] || die "no pod found for deployment $target_deployment"
kubectl -n cityevents delete pod "$pod_name" --wait=false

log "Wait for replacement readiness"
kubectl -n cityevents wait --for=condition=available "deployment/$target_deployment" --timeout=180s
ready_replicas="$(kubectl -n cityevents get "deployment/$target_deployment" -o jsonpath='{.status.readyReplicas}')"
[[ "${ready_replicas:-0}" -ge 1 ]] || die "$target_deployment did not recover a ready replica"

echo "Live pod-failure test completed for $target_deployment."
