# Kubernetes Live Smoke Testing

## Purpose

This test records whether CityEvents can run in a local Kubernetes cluster and
whether one selected app pod can be deleted and replaced.

It is not a production HA test. It uses local Minikube plus single-instance
backing services from `deploy/kubernetes/local`.

## Prerequisites

- Docker Desktop
- `kubectl`
- `minikube`
- Bash
- enough local resources for roughly 25 pods

## Static Gate

Run:

```bash
./scripts/verify-phase-14.sh
```

This checks:

- Phase 13 still passes
- the local overlay files exist
- local dependency manifests include Postgres, RabbitMQ, Redis, MinIO, and
  Mailpit
- the live script still builds/loads images, applies the overlay, runs
  migrations, port-forwards the gateway, and supports pod deletion
- `kubectl kustomize --load-restrictor=LoadRestrictionsNone deploy/kubernetes/local` passes when `kubectl` is present

## Live Gate

Run:

```bash
./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

The script writes evidence under:

```text
tmp/k8s-live-smoke/<timestamp>/
```

Important files:

- `summary.md`: scope, results, and caveats
- `current-context.txt`: Kubernetes context used
- `nodes.txt`: node inventory
- `deployments.txt`: deployment state after startup
- `pods-after-start.txt`: pod state after startup
- `pods-after-failure.txt`: pod state after deletion and recovery
- `deleted-pod.txt`: pod selected for deletion
- `metrics.txt`: gateway metrics sample

## What Pass Means

A passing live run supports:

```text
Local Kubernetes deployment smoke-tested with real service images, SQL migrations, gateway workflow verification, metrics verification, and one app pod replacement check.
```

It does not support:

```text
Production high availability.
```

## Current Environment Blocker

The live command was attempted on 2026-06-11, but Minikube failed before the app
was deployed:

```text
K8S_APISERVER_MISSING: apiserver process never appeared
```

The observed status was:

```text
host: Running
kubelet: Stopped
apiserver: Stopped
kubeconfig: Configured
```

Do not treat this as live Kubernetes evidence. The next safe remediation is to
recreate or repair the local Minikube profile, then rerun the same smoke command
and review the generated `summary.md`.

## Follow-Up Failure Tests

The next evidence level should add:

- continuous request load while deleting an API pod
- worker deletion during queued message processing
- RabbitMQ restart during outbox relay publishing
- Redis outage during feed reads and token-revocation checks
- Postgres outage and recovery behavior
- TLS ingress smoke with a real `cityevents-tls` secret
- multi-node scheduling evidence
