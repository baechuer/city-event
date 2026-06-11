# Kubernetes Live Smoke Testing

## Purpose

This test records whether CityEvents can run in a local Kubernetes cluster and
whether one selected app pod can be deleted and replaced.

It is not a production HA test. It uses Minikube plus single-instance
backing services from `deploy/kubernetes/local`.

It is also not a lightweight unit test. Minikube with the Docker driver runs on
the workstation's Docker/WSL runtime and can consume enough CPU, memory, and
network resources to destabilize a small local machine. For repeat evidence,
use the manual GitHub Actions `Heavy Evidence` workflow. The script is blocked
outside GitHub Actions so it cannot destabilize the local workstation.

## Prerequisites

- manual GitHub Actions `Heavy Evidence` workflow
- Docker available on the GitHub-hosted runner
- `minikube` installed by the workflow

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

The live gate is executed by `.github/workflows/heavy-evidence.yml`:

```bash
./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

Do not run this from the local workstation. `verify-phase-15.sh` keeps the live
path out of normal verification, checks that the smoke script calls the
GitHub-Actions-only evidence guard, and statically rejects host process-killing
cleanup.

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

## Current Environment Notes

The first live command attempted on 2026-06-11 failed before the app was
deployed:

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

Do not treat this as live Kubernetes evidence. The next accepted remediation is
to run the manual GitHub Actions `Heavy Evidence` workflow and review the
uploaded `summary.md`.

After profile repair, later runs reached dependency readiness, migrations, app
workload readiness, and gateway port-forwarding. They uncovered two Windows/Git
Bash transport issues:

- direct `kubectl.exe` from Bash was less stable than `minikube kubectl --`
- Windows `curl.exe` could reach the port-forward but needed request bodies
  posted from no-BOM temp files with converted Windows paths

The script now handles those transport issues and refuses to take over an
already-used local gateway port. Further live evidence must run in GitHub
Actions before it is treated as resume evidence.

## Follow-Up Failure Tests

The next evidence level should add:

- continuous request load while deleting an API pod
- worker deletion during queued message processing
- review the `dependency-failure-evidence` Actions artifact for Redis/RabbitMQ
  outage evidence, then repeat under higher volume
- Postgres outage and recovery behavior
- TLS ingress smoke with a real `cityevents-tls` secret
- multi-node scheduling evidence
