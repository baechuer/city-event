# Phase 14 Kubernetes Local Live Smoke And Failure Evidence

## Objective

Phase 14 moves Kubernetes from static manifest readiness toward runnable local
evidence.

The goal is not to claim production high availability. The goal is to prove that
the current manifests can run in a local Kubernetes cluster, that the gateway
workflow works after migrations, and that Kubernetes can replace one selected
app pod while the gateway becomes ready again.

## Implementation Map

| Capability | Files |
| --- | --- |
| Local Kubernetes overlay | `deploy/kubernetes/local/kustomization.yaml` |
| Local-only dependencies | `deploy/kubernetes/local/dependencies.yaml` |
| Local secret patch | `deploy/kubernetes/local/secret.local.patch.yaml` |
| Live smoke/failure runner | `scripts/k8s-live-smoke.sh` |
| Phase verifier | `scripts/verify-phase-14.sh` |
| Testing guide | `docs/testing/kubernetes-live-smoke.md` |
| CI gate | `.github/workflows/ci.yml` |

## Design Choice

The base Kubernetes manifests remain dependency-agnostic. They describe the app
workloads, services, ingress, probes, resources, replicas, and
PodDisruptionBudgets.

The new `deploy/kubernetes/local` overlay adds local-only backing services:

- Postgres
- RabbitMQ
- Redis
- MinIO
- Mailpit

This separation keeps the production path honest. A real deployment should use
managed or clustered backing services instead of copying the single-instance
local dependencies into production.

## Live Smoke Flow

`scripts/k8s-live-smoke.sh` performs the following:

- optionally starts Minikube with the Docker driver
- builds all CityEvents service images from the shared Dockerfile
- loads the images into Minikube
- applies `deploy/kubernetes/local`
- waits for Postgres, RabbitMQ, Redis, MinIO, and Mailpit
- applies all SQL migrations inside the Kubernetes Postgres pod
- restarts app workloads after migrations
- waits for all app deployments
- port-forwards `api-gateway`
- checks `/livez` and `/readyz`
- registers an organizer and attendee
- logs in as the seeded admin and promotes the organizer
- creates an event
- joins that event as the attendee
- verifies `/metrics`
- optionally deletes one app pod and waits for replacement

The generated evidence is written to `tmp/k8s-live-smoke/<timestamp>/`.

## Why This Still Is Not High Availability

This phase can support a stronger Kubernetes-readiness claim, but it still does
not prove production high availability.

Limitations:

- the local cluster is normally single-node Minikube
- the local overlay uses single-instance backing services
- there is no HorizontalPodAutoscaler
- there is no production ingress controller or real certificate evidence
- dependency failure tests are not included
- traffic continuity during deletion is checked through readiness after
  replacement, not a continuous external load stream

The app workloads have two replicas and PodDisruptionBudgets, but the full
system is only as available as Postgres, RabbitMQ, Redis, object storage, and
ingress.

## Verification

Static and CI-safe:

```bash
./scripts/verify-phase-14.sh
```

Live local smoke and pod replacement:

```bash
./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

Optional explicit verifier path:

```bash
./scripts/verify-phase-14.sh --run-live
```

## Current Local Run Result

On 2026-06-11, the static Phase 14 verifier passed locally. The live Minikube
smoke was attempted with:

```bash
./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

The script did not reach image builds or app deployment because Minikube failed
while starting the Kubernetes control plane:

```text
K8S_APISERVER_MISSING: apiserver process never appeared
```

`minikube status` reported:

```text
host: Running
kubelet: Stopped
apiserver: Stopped
kubeconfig: Configured
```

This means Phase 14 currently proves static/local-smoke readiness only. It does
not yet prove live Kubernetes app recovery in this environment.

## Claim Boundary

Allowed after static verification:

```text
Added a local Kubernetes overlay and CI-safe verifier for Minikube smoke-test readiness.
```

Allowed only after the live smoke succeeds:

```text
Ran a local Kubernetes smoke test that deployed the service set, applied migrations, verified the gateway event workflow, and confirmed Kubernetes replaced a selected app pod.
```

Still not allowed:

```text
Highly available production Kubernetes deployment.
```

```text
Production-grade HA Postgres/RabbitMQ/Redis.
```

```text
Autoscaled production deployment.
```
