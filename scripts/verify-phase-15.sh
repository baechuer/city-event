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
  .github/workflows/heavy-evidence.yml \
  deploy/observability/prometheus-rules.yaml \
  deploy/observability/grafana/cityevents-async-ops-dashboard.json \
  deploy/observability/otel-collector.yaml \
  deploy/kubernetes/hpa.yaml \
  deploy/kubernetes/network-policies.yaml \
  deploy/kubernetes/security-hardening.patch.yaml \
  deploy/kubernetes/topology-spread.patch.yaml \
  .github/workflows/security.yml \
  scripts/repair-minikube.sh \
  scripts/k8s-live-smoke.sh \
  scripts/failure-test-dependencies.sh \
  scripts/route-correctness-evidence.sh \
  scripts/security-error-evidence.sh \
  scripts/load-test-local.sh \
  scripts/load-sweep-local.sh \
  scripts/inspect-async-ops.sh \
  scripts/requeue-dead-outbox.sh \
  scripts/copy-rabbitmq-dlq.sh; do
  require_file "$file"
done

require_contains deploy/observability/prometheus-rules.yaml "CityEventsConsumerDeadLettered"
require_contains deploy/observability/prometheus-rules.yaml "CityEventsOutboxDeadRows"
require_contains deploy/observability/prometheus-rules.yaml "rabbitmq_queue_messages_ready"
require_contains deploy/observability/grafana/cityevents-async-ops-dashboard.json "CityEvents Async Operations"
require_contains deploy/observability/grafana/cityevents-async-ops-dashboard.json "RabbitMQ DLQ Depth (Exporter Required)"
require_contains deploy/observability/otel-collector.yaml "otlp"
require_contains deploy/observability/otel-collector.yaml "pipelines:"
require_contains .github/workflows/security.yml "govulncheck"
require_contains .github/workflows/security.yml "npm audit --omit=dev --audit-level=high"
require_contains .github/workflows/security.yml "github/codeql-action/analyze"
require_contains deploy/kubernetes/kustomization.yaml "hpa.yaml"
require_contains deploy/kubernetes/kustomization.yaml "network-policies.yaml"
require_contains deploy/kubernetes/kustomization.yaml "security-hardening.patch.yaml"
require_contains deploy/kubernetes/kustomization.yaml "topology-spread.patch.yaml"
require_contains deploy/kubernetes/local/kustomization.yaml "../hpa.yaml"
require_contains deploy/kubernetes/local/kustomization.yaml "../network-policies.yaml"
require_contains deploy/kubernetes/local/kustomization.yaml "../security-hardening.patch.yaml"
require_contains deploy/kubernetes/local/kustomization.yaml "../topology-spread.patch.yaml"
require_contains deploy/kubernetes/security-hardening.patch.yaml "automountServiceAccountToken: false"
require_contains deploy/kubernetes/security-hardening.patch.yaml "readOnlyRootFilesystem: true"
require_contains deploy/kubernetes/security-hardening.patch.yaml "drop:"
require_contains deploy/kubernetes/topology-spread.patch.yaml "topologySpreadConstraints"
require_contains deploy/kubernetes/hpa.yaml "HorizontalPodAutoscaler"
require_contains deploy/kubernetes/network-policies.yaml "default-deny"
require_contains .github/workflows/heavy-evidence.yml "profile: small"
require_contains .github/workflows/heavy-evidence.yml "profile: medium"
require_contains .github/workflows/heavy-evidence.yml "profile: 100-concurrent"
require_contains .github/workflows/heavy-evidence.yml "profile: stress"
require_contains .github/workflows/heavy-evidence.yml "load-evidence-\${{ matrix.profile }}-\${{ matrix.users }}u"
require_contains .github/workflows/heavy-evidence.yml "run_load_sweep"
require_contains .github/workflows/heavy-evidence.yml "load-sweep-evidence"
require_contains .github/workflows/heavy-evidence.yml "run_dependency_failures"
require_contains .github/workflows/heavy-evidence.yml "dependency-failure-evidence"
require_contains .github/workflows/heavy-evidence.yml "RATE_LIMIT_AUTH_REQUESTS: 1000"
require_contains .github/workflows/ci.yml "route-correctness-evidence"
require_contains .github/workflows/ci.yml "security-error-evidence"
require_contains scripts/repair-minikube.sh "delete_profile=true"
require_contains scripts/repair-minikube.sh "wait_for_cluster_access"
require_contains scripts/k8s-live-smoke.sh "A completed run is local Kubernetes evidence"
require_contains scripts/k8s-live-smoke.sh "dependency_images"
require_contains scripts/k8s-live-smoke.sh "Load dependency image into Minikube"
require_contains scripts/k8s-live-smoke.sh "ensure_local_port_free"
require_contains scripts/k8s-live-smoke.sh "require_github_actions_evidence_runner"
require_contains scripts/repair-minikube.sh "require_github_actions_evidence_runner"
require_contains scripts/load-test-local.sh "require_github_actions_evidence_runner"
require_contains scripts/load-sweep-local.sh "require_github_actions_evidence_runner"
require_contains scripts/failure-test-dependencies.sh "require_github_actions_evidence_runner"
require_contains scripts/failure-test-dependencies.sh "tmp/failure-tests"
require_contains scripts/failure-test-dependencies.sh "RabbitMQ outage scenario"
require_contains scripts/failure-test-dependencies.sh "inspect-async-ops.sh"
require_contains scripts/failure-test-dependencies.sh "metrics.tsv"
require_contains scripts/failure-test-dependencies.sh "rabbitmq_total_recovery_seconds"
require_contains scripts/failure-test-dependencies.sh "redis_restart_health_seconds"
require_contains scripts/failure-test-dependencies.sh "postgres_restart_health_seconds"
require_contains scripts/failure-test-dependencies.sh "postgres_outage_partial_user_rows"
require_contains scripts/failure-test-dependencies.sh "postgres-outage-no-partial-writes"
require_contains scripts/failure-test-dependencies.sh "__cityevents_exit_code"
require_contains scripts/load-sweep-local.sh "bash \"\$REPO_ROOT/scripts/load-test-local.sh\""
require_contains scripts/load-sweep-local.sh "wait_for_log_contains"
require_contains scripts/load-sweep-local.sh "CityEvents local stack is running."
require_contains scripts/route-correctness-evidence.sh "Route Correctness Evidence Summary"
require_contains scripts/route-correctness-evidence.sh "TestAuthHandlersRegisterValidationAndDuplicate"
require_contains scripts/route-correctness-evidence.sh "TestEventHandlersWorkflow"
require_contains scripts/route-correctness-evidence.sh "TestMediaHandlersWorkflow"
require_contains scripts/route-correctness-evidence.sh "TestGatewayValidatesTokenAndStripsSpoofedIdentityHeaders"
require_contains scripts/route-correctness-evidence.sh "TestOutboxAndConsumerMetrics"
require_contains scripts/security-error-evidence.sh "Security And Error Evidence Summary"
require_contains scripts/security-error-evidence.sh "TestAuthHandlersLoginFailures"
require_contains scripts/security-error-evidence.sh "TestDecodeJSONLimitedRejectsLargeBody"
require_contains scripts/security-error-evidence.sh "TestAuthHandlersRefreshRotatesCookieAndRejectsReuse"
require_contains scripts/security-error-evidence.sh "TestGatewayValidatesTokenAndStripsSpoofedIdentityHeaders"
require_contains scripts/security-error-evidence.sh "TestRateLimitRejectsRepeatedRequests"
require_contains scripts/security-error-evidence.sh "TestMetricsRequiresBearerTokenWhenConfigured"
require_contains scripts/load-test-local.sh "Diagnostic join latency p50 seconds"
require_contains scripts/load-test-local.sh "Join latency p99 seconds"
require_contains scripts/load-test-local.sh "Join throughput requests/second"
require_contains scripts/load-test-local.sh "metrics.tsv"
require_contains scripts/load-test-local.sh "Resource context status"
require_contains scripts/load-test-local.sh "nproc"
require_contains scripts/load-test-local.sh "lscpu"
require_contains scripts/load-test-local.sh "free -m"
require_contains scripts/load-test-local.sh "process-stats-before.tsv"
require_contains scripts/load-test-local.sh "highest_go_process_cpu_percent"
require_contains scripts/load-sweep-local.sh "Max stable RPS candidate"
require_contains scripts/load-sweep-local.sh "Highest passing load profile"
require_contains scripts/load-sweep-local.sh "Max stable RPS profile"
require_contains scripts/load-sweep-local.sh "resource_context_status"
require_contains scripts/load-sweep-local.sh "highest_go_process_rss_mb"
require_contains scripts/load-sweep-local.sh "Saturation point"
require_contains scripts/load-test-local.sh "dependency_snapshot"
require_contains scripts/load-test-local.sh "select status, count(*) from outbox_messages"
require_contains scripts/inspect-async-ops.sh "read-only"
require_contains scripts/requeue-dead-outbox.sh "CITYEVENTS_ALLOW_LOCAL_REPLAY"
require_contains scripts/copy-rabbitmq-dlq.sh "ack_requeue_true"
require_contains scripts/copy-rabbitmq-dlq.sh "CITYEVENTS_ALLOW_LOCAL_REPLAY"
require_contains scripts/k8s-live-smoke.sh "require_github_actions_evidence_runner"
require_contains scripts/load-test-local.sh "require_github_actions_evidence_runner"
require_contains scripts/load-sweep-local.sh "require_github_actions_evidence_runner"
require_contains scripts/failure-test-dependencies.sh "require_github_actions_evidence_runner"
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
