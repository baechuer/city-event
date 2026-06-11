#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

run_live=false

usage() {
  cat <<'EOF'
Usage: ./scripts/verify-phase-14.sh [options]

Verifies Phase 14 Kubernetes local live-smoke readiness. The default path is
static and CI-safe. Use --run-live to start/run the Minikube smoke manually.

Options:
  --run-live   Run ./scripts/k8s-live-smoke.sh --start-minikube --run-failure
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

log "Phase 13 baseline verification"
"$REPO_ROOT/scripts/verify-phase-13.sh"

log "Phase 14 file coverage"
for file in \
  deploy/kubernetes/local/kustomization.yaml \
  deploy/kubernetes/local/dependencies.yaml \
  deploy/kubernetes/local/secret.local.patch.yaml \
  scripts/k8s-live-smoke.sh \
  docs/architecture/phase-14-kubernetes-live-smoke.md \
  docs/testing/kubernetes-live-smoke.md; do
  require_file "$file"
done

require_contains deploy/kubernetes/local/kustomization.yaml "../deployments.yaml"
require_contains deploy/kubernetes/local/kustomization.yaml "dependencies.yaml"
require_contains deploy/kubernetes/local/secret.local.patch.yaml "AdminPass12345"
require_contains deploy/kubernetes/local/dependencies.yaml "postgres:16-alpine"
require_contains deploy/kubernetes/local/dependencies.yaml "rabbitmq:3.13-management-alpine"
require_contains deploy/kubernetes/local/dependencies.yaml "redis:7-alpine"
require_contains deploy/kubernetes/local/dependencies.yaml "minio/minio"
require_contains deploy/kubernetes/local/dependencies.yaml "axllent/mailpit"
require_contains scripts/k8s-live-smoke.sh "minikube image load"
require_contains scripts/k8s-live-smoke.sh "kubectl_cmd kustomize --load-restrictor=LoadRestrictionsNone deploy/kubernetes/local"
require_contains scripts/k8s-live-smoke.sh "psql -U cityevents -d cityevents"
require_contains scripts/k8s-live-smoke.sh "port-forward service/api-gateway"
require_contains scripts/k8s-live-smoke.sh "run_failure=true"
require_contains docs/architecture/phase-14-kubernetes-live-smoke.md "single-instance backing services"
require_contains docs/testing/kubernetes-live-smoke.md "not a production HA test"

log "Bash syntax"
bash -n scripts/*.sh scripts/lib/*.sh

if command -v kubectl >/dev/null 2>&1; then
  log "kubectl kustomize local overlay"
  kubectl kustomize --load-restrictor=LoadRestrictionsNone deploy/kubernetes/local >/dev/null
else
  echo "kubectl not found; skipped local overlay kustomize validation."
fi

if [[ "$run_live" == true ]]; then
  log "Live Minikube smoke"
  "$REPO_ROOT/scripts/k8s-live-smoke.sh" --start-minikube --run-failure
else
  cat <<'EOF'
Live Kubernetes smoke skipped. To run it locally:

  ./scripts/k8s-live-smoke.sh --start-minikube --run-failure

Only treat Kubernetes recovery as live evidence after that command succeeds and
the generated tmp/k8s-live-smoke/.../summary.md is reviewed.
EOF
fi

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 14 verification completed."
