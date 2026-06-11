#!/usr/bin/env bash

set -Eeuo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

profile="minikube"
delete_profile=false
evidence_dir=""

usage() {
  cat <<'EOF'
Usage: ./scripts/repair-minikube.sh [options]

Repairs a Minikube profile enough for CityEvents live smoke testing.
It records before/after status and verifies kubectl cluster access.

Options:
  --profile NAME       Minikube profile. Default: minikube
  --delete-profile     Delete and recreate the profile before starting
  --evidence-dir DIR   Evidence directory. Default: tmp/minikube-repair/<timestamp>
  -h, --help           Show this help

This script is approved only inside GitHub Actions. Deleting a profile removes
Kubernetes cluster state for that profile.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      [[ $# -ge 2 ]] || die "--profile requires a value"
      profile="$2"
      shift 2
      ;;
    --delete-profile)
      delete_profile=true
      shift
      ;;
    --evidence-dir)
      [[ $# -ge 2 ]] || die "--evidence-dir requires a value"
      evidence_dir="$2"
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
require_github_actions_evidence_runner "scripts/repair-minikube.sh"

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

minikube_bin="$(resolve_executable minikube)" || die "minikube was not found"

kubectl_cmd() {
  "$minikube_bin" -p "$profile" kubectl -- "$@"
}

if [[ -z "$evidence_dir" ]]; then
  evidence_dir="$REPO_ROOT/tmp/minikube-repair/$(date -u +%Y%m%dT%H%M%SZ)"
elif [[ "$evidence_dir" != /* ]]; then
  evidence_dir="$REPO_ROOT/$evidence_dir"
fi
mkdir -p "$evidence_dir"
summary_file="$evidence_dir/summary.md"

record() {
  printf -- '- %s\n' "$*" >>"$summary_file"
}

wait_for_cluster_access() {
  local timeout_seconds="${1:-420}"
  local deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    if kubectl_cmd get nodes --request-timeout=20s >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  kubectl_cmd get nodes --request-timeout=20s
}

profile_is_running() {
  local status
  status="$("$minikube_bin" -p "$profile" status 2>/dev/null || true)"
  [[ "$status" == *"host: Running"* && "$status" == *"kubelet: Running"* && "$status" == *"apiserver: Running"* ]]
}

on_error() {
  local exit_code=$?
  if [[ -f "$summary_file" ]]; then
    record "Failed before completion with exit code \`$exit_code\`."
  fi
}

trap on_error ERR

cat >"$summary_file" <<EOF
# Minikube Repair Evidence

- Date UTC: $(date -u +%Y-%m-%dT%H:%M:%SZ)
- Profile: \`$profile\`
- Delete profile: \`$delete_profile\`

## Results

EOF

log "Record Minikube state before repair"
"$minikube_bin" profile list >"$evidence_dir/profile-list-before.txt" 2>&1 || true
"$minikube_bin" -p "$profile" status >"$evidence_dir/status-before.txt" 2>&1 || true
record "Recorded profile and status before repair."

if [[ "$delete_profile" == true ]]; then
  log "Delete Minikube profile $profile"
  "$minikube_bin" -p "$profile" delete >"$evidence_dir/delete-profile.txt" 2>&1 || true
  record "Deleted profile \`$profile\` before recreation."
fi

if profile_is_running; then
  log "Minikube profile $profile already running"
  "$minikube_bin" -p "$profile" status >"$evidence_dir/start.txt" 2>&1
  record "Minikube profile was already running; start skipped."
else
  log "Start Minikube profile $profile"
  "$minikube_bin" -p "$profile" start --driver=docker --wait=apiserver,system_pods --wait-timeout=8m >"$evidence_dir/start.txt" 2>&1
  sleep 30
  record "Minikube start completed."
fi

log "Update kubectl context"
"$minikube_bin" -p "$profile" update-context >"$evidence_dir/update-context.txt" 2>&1
record "Updated kubectl context for profile \`$profile\`."

log "Verify cluster access"
kubectl_cmd config current-context >"$evidence_dir/current-context.txt"
sleep 10
wait_for_cluster_access 420
kubectl_cmd cluster-info --request-timeout=30s >"$evidence_dir/cluster-info.txt" 2>&1 || true
kubectl_cmd get nodes -o wide >"$evidence_dir/nodes.txt"
"$minikube_bin" -p "$profile" status >"$evidence_dir/status-after.txt" 2>&1
record "kubectl cluster access verified."

cat <<EOF

Minikube repair completed.

Evidence:
  $summary_file
EOF
