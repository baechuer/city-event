# Heavy Evidence Runner Policy

## Rule

Heavy evidence must not run from the local workstation.

The approved runner is GitHub Actions through the manual
`Heavy Evidence` workflow in `.github/workflows/heavy-evidence.yml`.

The load-evidence job intentionally runs as a GitHub Actions matrix:

| Profile | Users | Capacity | Join concurrency |
| --- | ---: | ---: | ---: |
| small | 40 | 15 | 10 |
| medium | 80 | 25 | 20 |
| stress | 160 | 50 | 40 |

## Protected Scripts

The following scripts are blocked outside GitHub Actions:

- `scripts/k8s-live-smoke.sh`
- `scripts/repair-minikube.sh`
- `scripts/failure-test-kubernetes.sh --live`
- `scripts/load-test-local.sh`
- `scripts/failure-test-dependencies.sh`

Static verification scripts may still run locally. They must not start
Minikube, mutate a Kubernetes cluster, run a gateway load test, or stress Docker
Desktop.

## Reason

Minikube, Docker Desktop, WSL networking, multiple service images, Kubernetes
pods, and concurrent load tests are host-level operations. They can consume
enough CPU, memory, disk, and networking resources to destabilize a developer
machine.

The project keeps these tests as evidence, but they belong on an isolated CI
runner where failure does not affect the workstation.

## Acceptance Evidence

Before a heavy-evidence claim is resume-safe, capture:

- GitHub Actions run URL
- commit SHA
- exact workflow inputs
- uploaded evidence artifact
- generated `summary.md`
- dependency snapshots from the load evidence artifact, when present
- Redis/RabbitMQ failure scenario results from the dependency failure artifact,
  when present
- whether the result changes a resume-safe claim

## Local Safety Gate

`scripts/lib/common.sh` exposes
`require_github_actions_evidence_runner`. Heavy scripts must call it before
starting Docker, Minikube, Kubernetes mutation, or load traffic.

`scripts/verify-phase-15.sh` statically verifies this policy and rejects host
process-killing cleanup in the Kubernetes smoke script.
