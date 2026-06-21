# Kubernetes Readiness

This directory contains Kubernetes manifests for deploying the CityEvents
services and workers. The manifests are intended to demonstrate deployment
readiness, configuration separation, security hardening, and failure-test
scaffolding.

## What Is Included

| File | Purpose |
| --- | --- |
| `namespace.yaml` | `cityevents` namespace. |
| `configmap.yaml` | Non-secret runtime configuration, service URLs, CORS, rate-limit settings. |
| `secret.example.yaml` | Example secret values for local/staging replacement. Do not use as production secrets. |
| `deployments.yaml` | App service and worker deployments. |
| `services.yaml` | ClusterIP services for gateway and internal services. |
| `ingress.yaml` | Public HTTP entry through the API gateway with TLS reference. |
| `poddisruptionbudgets.yaml` | Minimum availability intent during voluntary disruption. |
| `hpa.yaml` | CPU-based autoscaling intent. Requires metrics-server. |
| `security-hardening.patch.yaml` | Non-root pods, dropped capabilities, read-only root filesystem, seccomp, no service account token automount. |
| `topology-spread.patch.yaml` | Preferred spreading across hosts/zones while staying schedulable in Minikube. |
| `network-policies.yaml` | Default deny, DNS egress, ingress-controller to gateway, same-namespace traffic. |
| `cert-manager-certificate.example.yaml` | Example cert-manager `Certificate` for the ingress TLS secret. |
| `local/` | Local Minikube overlay with single-instance development dependencies and local secrets. |

## Public Entry Point

The intended public entry is the ingress -> API gateway path:

```text
Browser
  -> https://cityevents.local/v1/...
  -> ingress controller
  -> api-gateway service
  -> internal services
```

The frontend should point its runtime `CITYEVENTS_API_BASE` at the ingress
origin. Internal services are not meant to be browser entry points.

`ingress.yaml` routes:

- `/v1/*` to `api-gateway`
- `/readyz` to `api-gateway`
- `/livez` to `api-gateway`

`/metrics` is intentionally not exposed publicly. Metrics should be scraped
inside the cluster.

## Build Images

Build one image per service or worker using the shared Dockerfile:

```bash
docker build --build-arg SERVICE=api-gateway -t cityevents/api-gateway:dev .
docker build --build-arg SERVICE=auth-service -t cityevents/auth-service:dev .
docker build --build-arg SERVICE=event-registration-service -t cityevents/event-registration-service:dev .
docker build --build-arg SERVICE=feed-service -t cityevents/feed-service:dev .
docker build --build-arg SERVICE=notification-service -t cityevents/notification-service:dev .
docker build --build-arg SERVICE=media-service -t cityevents/media-service:dev .
docker build --build-arg SERVICE=outbox-relay -t cityevents/outbox-relay:dev .
docker build --build-arg SERVICE=feed-worker -t cityevents/feed-worker:dev .
docker build --build-arg SERVICE=notification-worker -t cityevents/notification-worker:dev .
docker build --build-arg SERVICE=media-worker -t cityevents/media-worker:dev .
```

The CI workflow builds these images as a matrix.

## Configure Secrets

Replace `secret.example.yaml` before any real deployment.

At minimum, provide production-safe values for:

- `POSTGRES_URL`
- `RABBITMQ_URL`
- `REDIS_URL`
- `MINIO_ACCESS_KEY`
- `MINIO_SECRET_KEY`
- `JWT_SECRET`
- seed admin values, if seeding is enabled

For production, prefer external secret management instead of committing secret
YAML.

## TLS And Cert Manager

`ingress.yaml` references a TLS secret named `cityevents-tls`.

For a local `cityevents.local` host, create it manually:

```bash
kubectl create secret tls cityevents-tls \
  --namespace cityevents \
  --cert path/to/tls.crt \
  --key path/to/tls.key
```

For a real public domain, install cert-manager, create a production
`ClusterIssuer`, update `cert-manager-certificate.example.yaml`, and apply it:

```bash
kubectl apply -f deploy/kubernetes/cert-manager-certificate.example.yaml
```

Cert-manager should then create and renew the `cityevents-tls` secret that the
ingress references.

## Apply The Base Manifests

```bash
kubectl apply -k deploy/kubernetes
```

The base manifests assume backing services already exist. They do not create
PostgreSQL, RabbitMQ, Redis, MinIO, or SMTP infrastructure.

## Local Minikube Overlay

The local overlay adds development-only single-instance dependencies and
development secrets:

```bash
kubectl apply -k deploy/kubernetes/local
```

For automated local Kubernetes evidence, use:

```bash
./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

This script is guarded for GitHub Actions heavy-evidence runs because it can be
host-intensive. It builds/loads images, applies the local overlay, checks
gateway readiness, exercises the API path, and can delete a pod to observe
replacement.

## Hardening Demonstrated

- Two replicas per app workload.
- Readiness and liveness probes.
- Resource requests and limits.
- PodDisruptionBudgets.
- Security contexts with non-root execution.
- Dropped Linux capabilities.
- Read-only root filesystem with writable temporary paths.
- RuntimeDefault seccomp profile.
- Disabled service account token automount.
- HPA manifests for autoscaling intent.
- Topology spread constraints.
- NetworkPolicies for default deny and controlled access.
- HTTPS ingress configuration and cert-manager example.

## Production Evolution

Ingress plus replicas are an important deployment foundation. A production
environment would build on these manifests with live multi-node evidence and
managed backing services.

The next production steps are:

- A real multi-node cluster.
- Evidence that replicas schedule across nodes.
- Metrics-server-backed HPA behavior under load.
- Continuous traffic while pods are deleted or nodes fail.
- Highly available PostgreSQL, RabbitMQ, Redis, object storage, and SMTP/provider
  dependencies.
- Persistent volume and backup/restore strategy.
- Secrets managed outside Git.
- Production observability stack with alert delivery.
- Release and rollback procedure.

## Verification

Static readiness:

```bash
bash ./scripts/verify-phase-10.sh
bash ./scripts/verify-phase-14.sh
bash ./scripts/verify-phase-15.sh
```

Kubernetes failure-test script:

```bash
bash ./scripts/failure-test-kubernetes.sh
```

Live pod-deletion test, only after confirming the current `kubectl` context is
safe:

```bash
bash ./scripts/failure-test-kubernetes.sh --live --deployment api-gateway
```

Manual heavy-evidence GitHub Actions workflow:

```text
Actions -> Heavy Evidence -> Run workflow
```

That workflow can run Minikube smoke, load evidence, and dependency-failure
evidence, uploading artifacts under `tmp/...`.

## How I Present This

These manifests demonstrate:

```text
Kubernetes-ready manifests with replicated workloads, probes, resource limits,
ConfigMap/Secret separation, TLS ingress, cert-manager example,
PodDisruptionBudgets, HPA intent, topology spread, security contexts, and
NetworkPolicies.
```

The production evolution is:

```text
run the same architecture on a real cluster with managed backing services,
autoscaling evidence, multi-node failure evidence, and live observability.
```
