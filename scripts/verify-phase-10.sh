#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"
setup_go_cache

log "Default Go tests"
run_go test ./...

log "Docker Compose config"
run_docker compose config --quiet

log "Kubernetes manifest static checks"
for file in \
  Dockerfile \
  .dockerignore \
  deploy/kubernetes/README.md \
  deploy/kubernetes/namespace.yaml \
  deploy/kubernetes/configmap.yaml \
  deploy/kubernetes/secret.example.yaml \
  deploy/kubernetes/deployments.yaml \
  deploy/kubernetes/services.yaml \
  deploy/kubernetes/ingress.yaml \
  deploy/kubernetes/kustomization.yaml; do
  require_file "$file"
done

require_contains Dockerfile "ARG SERVICE=api-gateway"
require_contains Dockerfile "go build"

http_services=(api-gateway auth-service event-registration-service feed-service notification-service media-service)
workers=(feed-worker notification-worker media-worker outbox-relay)
all_deployments=("${http_services[@]}" "${workers[@]}")

for name in "${all_deployments[@]}"; do
  require_contains deploy/kubernetes/deployments.yaml "name: $name"
  require_contains deploy/kubernetes/deployments.yaml "image: cityevents/$name:dev"
done

for name in "${http_services[@]}"; do
  require_contains deploy/kubernetes/services.yaml "name: $name"
done

[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "readinessProbe:")" -ge "${#all_deployments[@]}" ]] || die "each deployment must define a readinessProbe."
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "livenessProbe:")" -ge "${#all_deployments[@]}" ]] || die "each deployment must define a livenessProbe."
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "resources:")" -ge "${#all_deployments[@]}" ]] || die "each deployment must define resource requests and limits."
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "requests:")" -ge "${#all_deployments[@]}" ]] || die "each deployment must define resource requests."
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "limits:")" -ge "${#all_deployments[@]}" ]] || die "each deployment must define resource limits."
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "/readyz")" -ge "${#http_services[@]}" ]] || die "each HTTP service must use /readyz readiness probe."
[[ "$(count_occurrences deploy/kubernetes/deployments.yaml "/livez")" -ge "${#http_services[@]}" ]] || die "each HTTP service must use /livez liveness probe."

require_contains deploy/kubernetes/configmap.yaml "CITYEVENTS_ENV"
require_contains deploy/kubernetes/configmap.yaml "HTTP_ADDR"
require_contains deploy/kubernetes/configmap.yaml "AUTH_SERVICE_URL"
require_contains deploy/kubernetes/configmap.yaml "EVENT_SERVICE_URL"
require_contains deploy/kubernetes/configmap.yaml "FEED_SERVICE_URL"
require_contains deploy/kubernetes/configmap.yaml "MEDIA_SERVICE_URL"
require_contains deploy/kubernetes/secret.example.yaml "POSTGRES_URL"
require_contains deploy/kubernetes/secret.example.yaml "RABBITMQ_URL"
require_contains deploy/kubernetes/secret.example.yaml "JWT_SECRET"
require_contains deploy/kubernetes/secret.example.yaml "SEED_ADMIN_EMAIL"
require_contains deploy/kubernetes/ingress.yaml "name: cityevents-api"
require_contains deploy/kubernetes/ingress.yaml "name: api-gateway"

if command -v kubectl >/dev/null 2>&1; then
  log "kubectl kustomize"
  kubectl kustomize deploy/kubernetes >/dev/null
  if kubectl cluster-info --request-timeout=3s >/dev/null 2>&1; then
    log "kubectl client dry-run"
    kubectl apply --dry-run=client --validate=false -k deploy/kubernetes >/dev/null
  else
    echo "kubectl cluster is not reachable; skipped client dry-run."
  fi
else
  echo "kubectl not found; skipped client dry-run."
fi

echo "Phase 10 verification completed."
