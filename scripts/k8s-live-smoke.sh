#!/usr/bin/env bash

set -Eeuo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

start_minikube=false
skip_build=false
skip_load=false
run_failure=false
cleanup_namespace=false
target_deployment="api-gateway"
local_port="18080"
evidence_dir=""
original_args="$*"

app_deployments=(
  "api-gateway"
  "auth-service"
  "event-registration-service"
  "feed-service"
  "notification-service"
  "media-service"
  "outbox-relay"
  "feed-worker"
  "notification-worker"
  "media-worker"
)

dependency_deployments=(
  "postgres"
  "rabbitmq"
  "redis"
  "minio"
  "mailpit"
)

dependency_images=(
  "postgres:16-alpine"
  "rabbitmq:3.13-management-alpine"
  "redis:7-alpine"
  "minio/minio:latest"
  "axllent/mailpit:latest"
)

migrations=(
  "migrations/auth/001_init.sql"
  "migrations/eventregistration/001_init.sql"
  "migrations/eventregistration/002_outbox_relay.sql"
  "migrations/feed/001_init.sql"
  "migrations/notification/001_init.sql"
  "migrations/media/001_init.sql"
)

usage() {
  cat <<'EOF'
Usage: ./scripts/k8s-live-smoke.sh [options]

Runs a GitHub Actions-only Kubernetes smoke test using the local overlay:
  - optionally starts Minikube
  - builds service images with the shared Dockerfile
  - loads images into Minikube
  - applies deploy/kubernetes/local
  - runs Postgres migrations inside the Postgres pod
  - verifies gateway health, admin login, organizer promotion, event creation,
    attendee join, and metrics
  - optionally deletes one app pod and verifies recovery

Options:
  --start-minikube       Start Minikube with the Docker driver before running
  --skip-build           Reuse existing cityevents/*:dev images
  --skip-load            Do not run minikube image load
  --run-failure          Delete one pod after smoke verification
  --deployment NAME      Deployment to test with --run-failure. Default: api-gateway
  --port PORT            Local port for api-gateway port-forward. Default: 18080
  --evidence-dir DIR     Evidence output directory. Default: tmp/k8s-live-smoke/<timestamp>
  --cleanup              Delete the cityevents namespace at the end after success
  -h, --help             Show this help

This script is heavy host-level evidence and is approved only inside GitHub
Actions. It is not a production HA test because the local overlay uses
single-instance Postgres, RabbitMQ, Redis, MinIO, and Mailpit dependencies.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --start-minikube)
      start_minikube=true
      shift
      ;;
    --skip-build)
      skip_build=true
      shift
      ;;
    --skip-load)
      skip_load=true
      shift
      ;;
    --run-failure)
      run_failure=true
      shift
      ;;
    --deployment)
      [[ $# -ge 2 ]] || die "--deployment requires a value"
      target_deployment="$2"
      shift 2
      ;;
    --port)
      [[ $# -ge 2 ]] || die "--port requires a value"
      local_port="$2"
      shift 2
      ;;
    --evidence-dir)
      [[ $# -ge 2 ]] || die "--evidence-dir requires a value"
      evidence_dir="$2"
      shift 2
      ;;
    --cleanup)
      cleanup_namespace=true
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
require_github_actions_evidence_runner "scripts/k8s-live-smoke.sh"
setup_go_cache

if [[ -z "$evidence_dir" ]]; then
  evidence_dir="$REPO_ROOT/tmp/k8s-live-smoke/$(date -u +%Y%m%dT%H%M%SZ)"
elif [[ "$evidence_dir" != /* ]]; then
  evidence_dir="$REPO_ROOT/$evidence_dir"
fi
mkdir -p "$evidence_dir"
summary_file="$evidence_dir/summary.md"
port_forward_log="$evidence_dir/api-gateway-port-forward.log"
port_forward_pid=""
failure_recorded=false

cleanup() {
  cleanup_pid "$port_forward_pid"
}

on_error() {
  local exit_code=$?
  if [[ "$failure_recorded" == true ]]; then
    return
  fi
  failure_recorded=true
  if [[ -n "${summary_file:-}" && -f "$summary_file" ]]; then
    printf -- '- Failed before completion with exit code `%s`. Check the command output and cluster status before treating this as live evidence.\n' "$exit_code" >>"$summary_file"
  fi
}

trap cleanup EXIT
trap on_error ERR

resolve_executable() {
  local name="$1"
  local candidate
  for candidate in \
    "$(command -v "$name" 2>/dev/null || true)" \
    "$(command -v "$name.exe" 2>/dev/null || true)" \
    "/mnt/d/myplayground/Minikube/$name.exe" \
    "/d/myplayground/Minikube/$name.exe"; do
    if [[ -n "$candidate" && -x "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
  done
  return 1
}

kubectl_cmd() {
  minikube_cmd kubectl -- "$@"
}

minikube_cmd() {
  "$minikube_bin" "$@"
}

curl_file_path() {
  local path="$1"
  if [[ "${use_windows_curl:-false}" == true ]]; then
    if command -v wslpath >/dev/null 2>&1; then
      wslpath -w "$path"
      return
    fi
    if command -v cygpath >/dev/null 2>&1; then
      cygpath -w "$path"
      return
    fi
  fi
  printf '%s\n' "$path"
}

json_eval() {
  local expression="$1"
  local json="$2"
  JSON_INPUT="$json" run_node -e "const data = JSON.parse(process.env.JSON_INPUT || '{}'); const value = $expression; if (value === undefined || value === null || value === '') process.exit(2); process.stdout.write(String(value));"
}

curl_cmd() {
  if [[ "${use_windows_curl:-false}" == true ]]; then
    curl.exe "$@"
  else
    curl "$@"
  fi
}

wait_for_gateway_http() {
  local url="$1"
  local timeout_seconds="${2:-60}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if curl_cmd -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

ensure_local_port_free() {
  local port="$1"
  if command -v powershell.exe >/dev/null 2>&1; then
    if powershell.exe -NoProfile -Command "
      \$port = [int]'$port'
      \$listeners = Get-NetTCPConnection -LocalPort \$port -State Listen -ErrorAction SilentlyContinue
      if (\$listeners) { exit 1 }
    " >/dev/null 2>&1; then
      return 0
    fi
    die "local port $port is already in use. Stop the existing listener or pass --port with a free port."
  fi

  if command -v ss >/dev/null 2>&1; then
    if ss -ltn "sport = :$port" | tail -n +2 | grep -q .; then
      die "local port $port is already in use. Stop the existing listener or pass --port with a free port."
    fi
  fi
}

post_json() {
  local path="$1"
  local body="$2"
  local body_file
  local curl_body_file
  local status
  shift 2
  body_file="$(mktemp "$evidence_dir/request-body.XXXXXX.json")"
  printf '%s' "$body" >"$body_file"
  curl_body_file="$(curl_file_path "$body_file")"
  set +e
  curl_cmd -fsS \
    -H "Content-Type: application/json" \
    "$@" \
    --data-binary "@$curl_body_file" \
    "$api_base$path"
  status=$?
  set -e
  rm -f "$body_file"
  return "$status"
}

patch_json() {
  local path="$1"
  local body="$2"
  local body_file
  local curl_body_file
  local status
  shift 2
  body_file="$(mktemp "$evidence_dir/request-body.XXXXXX.json")"
  printf '%s' "$body" >"$body_file"
  curl_body_file="$(curl_file_path "$body_file")"
  set +e
  curl_cmd -fsS \
    -X PATCH \
    -H "Content-Type: application/json" \
    "$@" \
    --data-binary "@$curl_body_file" \
    "$api_base$path"
  status=$?
  set -e
  rm -f "$body_file"
  return "$status"
}

wait_for_deployment() {
  local name="$1"
  kubectl_cmd -n cityevents wait --for=condition=available "deployment/$name" --timeout=240s
}

wait_for_cluster_access() {
  local timeout_seconds="${1:-180}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if kubectl_cmd get nodes --request-timeout=20s >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  kubectl_cmd get nodes --request-timeout=20s
}

append_summary_header() {
  cat >"$summary_file" <<EOF
# Kubernetes Live Smoke Evidence

- Date UTC: $(date -u +%Y-%m-%dT%H:%M:%SZ)
- Command: \`$0 $*\`
- Overlay: \`deploy/kubernetes/local\`
- Local gateway: \`http://127.0.0.1:$local_port\`
- Failure test: \`$run_failure\`
- Failure deployment: \`$target_deployment\`

## Scope

A completed run is local Kubernetes evidence. It proves that the manifests can
be applied to a reachable cluster, app images can run, migrations can be
applied, the gateway workflow works, and Kubernetes can replace one selected pod
when \`--run-failure\` is used.

It does not prove production high availability because the local overlay runs
single-instance backing services.

## Results

EOF
}

record_result() {
  printf -- '- %s\n' "$*" >>"$summary_file"
}

minikube_bin="$(resolve_executable minikube)" || die "minikube was not found"
command -v docker >/dev/null 2>&1 || die "docker was not found"
command -v curl >/dev/null 2>&1 || die "curl was not found"
use_windows_curl=false
if command -v curl.exe >/dev/null 2>&1 && command -v wslpath >/dev/null 2>&1; then
  use_windows_curl=true
fi

append_summary_header "$original_args"

if [[ "$start_minikube" == true ]]; then
  log "Start Minikube"
  minikube_cmd start --driver=docker
  record_result "Minikube start completed."
fi

log "Check Kubernetes cluster access"
wait_for_cluster_access 180
kubectl_cmd cluster-info --request-timeout=30s >"$evidence_dir/cluster-info.txt" 2>&1 || true
kubectl_cmd config current-context >"$evidence_dir/current-context.txt"
kubectl_cmd get nodes -o wide >"$evidence_dir/nodes.txt"
record_result "Kubernetes cluster is reachable. Context recorded in \`current-context.txt\`."

if [[ "$skip_build" == false ]]; then
  for service in "${app_deployments[@]}"; do
    log "Build image $service"
    run_docker build --build-arg "SERVICE=$service" -t "cityevents/$service:dev" .
  done
  record_result "Built all CityEvents service images."
fi

if [[ "$skip_load" == false ]]; then
  for image in "${dependency_images[@]}"; do
    log "Pull dependency image $image"
    run_docker pull "$image"
    log "Load dependency image into Minikube $image"
    minikube_cmd image load "$image"
  done
  record_result "Loaded dependency images into Minikube."

  for service in "${app_deployments[@]}"; do
    log "Load image into Minikube $service"
    minikube_cmd image load "cityevents/$service:dev"
  done
  record_result "Loaded service images into Minikube."
fi

log "Apply local Kubernetes overlay"
kubectl_cmd kustomize --load-restrictor=LoadRestrictionsNone deploy/kubernetes/local |
  kubectl_cmd apply -f - >"$evidence_dir/apply.txt"
record_result "Applied \`deploy/kubernetes/local\`."

for deployment in "${dependency_deployments[@]}"; do
  log "Wait for dependency $deployment"
  wait_for_deployment "$deployment"
done
record_result "Dependency deployments became available."

log "Apply Postgres migrations in Kubernetes"
for migration in "${migrations[@]}"; do
  require_file "$REPO_ROOT/$migration"
  echo "Apply $migration" | tee -a "$evidence_dir/migrations.txt"
  kubectl_cmd -n cityevents exec -i deployment/postgres -- \
    psql -U cityevents -d cityevents -v ON_ERROR_STOP=1 -f - \
    <"$REPO_ROOT/$migration" >>"$evidence_dir/migrations.txt"
done
record_result "Applied all service migrations inside the Kubernetes Postgres pod."

log "Restart app workloads after migrations"
for deployment in "${app_deployments[@]}"; do
  kubectl_cmd -n cityevents rollout restart "deployment/$deployment" >/dev/null
done

for deployment in "${app_deployments[@]}"; do
  log "Wait for app $deployment"
  wait_for_deployment "$deployment"
done
kubectl_cmd -n cityevents get deployments -o wide >"$evidence_dir/deployments.txt"
kubectl_cmd -n cityevents get pods -o wide >"$evidence_dir/pods-after-start.txt"
record_result "All app deployments became available after migrations."

log "Port-forward api-gateway"
ensure_local_port_free "$local_port"
: >"$port_forward_log"
kubectl_cmd -n cityevents port-forward service/api-gateway "$local_port:80" >"$port_forward_log" 2>&1 &
port_forward_pid=$!
if ! wait_for_gateway_http "http://127.0.0.1:$local_port/readyz" 60; then
  tail -n 80 "$port_forward_log" >&2 || true
  die "api-gateway port-forward did not become ready"
fi
api_base="http://127.0.0.1:$local_port"
record_result "Gateway port-forward became ready."

log "Smoke gateway workflow"
curl_cmd -fsS "$api_base/livez" >/dev/null
curl_cmd -fsS "$api_base/readyz" >/dev/null

suffix="$(date +%s)"
organizer_email="k8s-organizer-$suffix@example.com"
attendee_email="k8s-attendee-$suffix@example.com"
password="StrongerPass123"
starts_at="$(run_node -e "process.stdout.write(new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString())")"

organizer_json="$(post_json "/v1/auth/register" "{\"email\":\"$organizer_email\",\"displayName\":\"K8s Organizer\",\"password\":\"$password\"}")"
attendee_json="$(post_json "/v1/auth/register" "{\"email\":\"$attendee_email\",\"displayName\":\"K8s Attendee\",\"password\":\"$password\"}")"
organizer_id="$(json_eval "data.user.id" "$organizer_json")"
attendee_token="$(json_eval "data.accessToken" "$attendee_json")"

admin_json="$(post_json "/v1/auth/login" '{"email":"admin@cityevents.local","password":"AdminPass12345"}')"
admin_token="$(json_eval "data.accessToken" "$admin_json")"
patch_json "/v1/auth/users/$organizer_id/role" '{"role":"ORGANIZER"}' -H "Authorization: Bearer $admin_token" >/dev/null

organizer_login_json="$(post_json "/v1/auth/login" "{\"email\":\"$organizer_email\",\"password\":\"$password\"}")"
organizer_token="$(json_eval "data.accessToken" "$organizer_login_json")"
event_json="$(post_json "/v1/events" "{\"title\":\"K8s Smoke Event $suffix\",\"description\":\"Kubernetes smoke test event\",\"city\":\"Sydney\",\"venue\":\"Minikube Hall\",\"startsAt\":\"$starts_at\",\"capacity\":3}" -H "Authorization: Bearer $organizer_token")"
event_id="$(json_eval "data.event.id" "$event_json")"

post_json "/v1/events/$event_id/join" '{}' -H "Authorization: Bearer $attendee_token" -H "Idempotency-Key: k8s-smoke-$suffix" >/dev/null
curl_cmd -fsS -H "Authorization: Bearer $attendee_token" "$api_base/v1/events/$event_id/join" >"$evidence_dir/join-status.json"
curl_cmd -fsS "$api_base/metrics" >"$evidence_dir/metrics.txt"
grep -Fq "cityevents_http_request_duration_seconds" "$evidence_dir/metrics.txt" || die "metrics endpoint did not expose request duration histogram"
record_result "Gateway workflow passed: register users, admin promotion, event creation, attendee join, and metrics."

if [[ "$run_failure" == true ]]; then
  log "Delete one pod for $target_deployment"
  pod_name="$(kubectl_cmd -n cityevents get pods \
    -l "app.kubernetes.io/component=$target_deployment" \
    -o jsonpath='{.items[0].metadata.name}')"
  [[ -n "$pod_name" ]] || die "no pod found for deployment $target_deployment"
  printf '%s\n' "$pod_name" >"$evidence_dir/deleted-pod.txt"
  kubectl_cmd -n cityevents delete pod "$pod_name" --wait=false
  wait_for_deployment "$target_deployment"
  if ! wait_for_gateway_http "$api_base/readyz" 60; then
    die "gateway readiness failed after pod deletion"
  fi
  kubectl_cmd -n cityevents get pods -o wide >"$evidence_dir/pods-after-failure.txt"
  record_result "Deleted pod \`$pod_name\` from \`$target_deployment\`; deployment became available and gateway readiness still passed."
fi

if [[ "$cleanup_namespace" == true ]]; then
  log "Delete cityevents namespace"
  kubectl_cmd delete namespace cityevents --wait=false >/dev/null
  record_result "Cleanup requested; deleted the \`cityevents\` namespace asynchronously."
fi

cat <<EOF

Kubernetes live smoke completed.

Evidence:
  $summary_file

Gateway:
  $api_base
EOF
