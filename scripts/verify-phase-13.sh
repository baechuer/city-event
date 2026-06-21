#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

run_e2e=false

usage() {
  cat <<'EOF'
Usage: ./scripts/verify-phase-13.sh [options]

Verifies Phase 13 CI/CD, browser E2E, rate limiting, observability, and
replica/failure-test readiness. Browser E2E is optional because it starts the
full local stack.

Options:
  --run-e2e    Run Playwright browser E2E after static and unit checks
  -h, --help   Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --run-e2e)
      run_e2e=true
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
setup_go_cache

log "Phase 12 baseline verification"
"$REPO_ROOT/scripts/verify-phase-12.sh"

log "Phase 13 file coverage"
for file in \
  .github/workflows/ci.yml \
  frontend/package.json \
  frontend/package-lock.json \
  frontend/playwright.config.mjs \
  frontend/e2e/cityevents.spec.mjs \
  internal/platform/httpapi/rate_limit.go \
  deploy/kubernetes/poddisruptionbudgets.yaml \
  scripts/failure-test-kubernetes.sh; do
  require_file "$file"
done

require_contains .github/workflows/ci.yml "browser-e2e"
require_contains .github/workflows/ci.yml "container-build"
require_contains .github/workflows/ci.yml "verify-phase-13.sh"
require_contains frontend/package.json "\"e2e\": \"playwright test\""
require_contains frontend/playwright.config.mjs "start-local.sh"
require_contains frontend/e2e/cityevents.spec.mjs "organizer publishes an event and an attendee joins it"
require_contains internal/platform/httpapi/rate_limit.go "rateLimitMiddleware"
require_contains internal/platform/observability/observability.go "cityevents_http_request_duration_seconds"
require_contains internal/platform/observability/observability.go "cityevents_rate_limited_requests_total"
require_contains deploy/kubernetes/configmap.yaml "RATE_LIMIT_ENABLED"
require_contains deploy/kubernetes/poddisruptionbudgets.yaml "kind: PodDisruptionBudget"

log "Kubernetes failure-test static gate"
bash "$REPO_ROOT/scripts/failure-test-kubernetes.sh"

log "Frontend unit checks"
(cd frontend && run_npm run verify)

if [[ "$run_e2e" == true ]]; then
  log "Playwright browser E2E"
  (cd frontend && run_npm run e2e)
else
  echo "Playwright E2E skipped. Run ./scripts/verify-phase-13.sh --run-e2e to start the local stack and execute it."
fi

log "Native smoke"
run_git -c core.autocrlf=false diff --check

echo "Phase 13 verification completed."
