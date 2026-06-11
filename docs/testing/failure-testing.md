# Failure Testing

## Purpose

Failure testing prevents the project from overclaiming high availability. The current repo now has replica and PodDisruptionBudget manifests, but a real HA claim still requires live failure evidence.

Live failure evidence is GitHub Actions-only. Do not run cluster mutation from
the local workstation. See `docs/testing/heavy-evidence-runner-policy.md`.

## Static Gate

Run:

```bash
bash ./scripts/failure-test-kubernetes.sh
```

This verifies:

- every Deployment declares `replicas: 2`
- every workload has a matching `PodDisruptionBudget`
- `poddisruptionbudgets.yaml` is included by Kustomize
- `kubectl kustomize deploy/kubernetes` succeeds when `kubectl` is available

The static gate does not create or modify a cluster.

## Live Pod-Failure Test

The live command is approved only inside the manual GitHub Actions
`Heavy Evidence` workflow or a future Actions job with equivalent isolation:

```bash
bash ./scripts/failure-test-kubernetes.sh --live --deployment api-gateway
```

The live path:

- checks cluster access
- applies `deploy/kubernetes`
- waits for Deployments to become available
- deletes one pod from the selected Deployment
- waits for Kubernetes to report the Deployment available again

For an all-in-one Minikube smoke that also provisions local dependencies and
runs the gateway workflow, the Actions workflow runs:

```bash
bash ./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

That command is documented in `docs/testing/kubernetes-live-smoke.md`.

## Required Evidence Before Claiming HA

Record the following before using high-availability wording on a resume:

- date and environment
- cluster type and node count
- image versions or commit SHA
- exact command used
- pod deleted and replacement timing
- request behavior during pod deletion
- dependency state for Postgres, RabbitMQ, Redis, and MinIO

## Remaining Failure Tests

Still required for a strong claim:

- API pod deletion while traffic continues
- worker pod deletion during message processing
- RabbitMQ restart during outbox relay publishing
- Redis outage during feed reads and token-revocation checks
- Postgres outage and recovery behavior
- concurrent joins above capacity during or after worker failure

The next hardening phase is governed by
`docs/architecture/phase-15-hardening-rubric.md`. A failure scenario does not
count as done unless it records exact commands, observed behavior, and whether
the result changes any resume-safe claim.

## Claim Boundary

Allowed:

```text
Added replicated Kubernetes manifests, PodDisruptionBudgets, and a guarded pod-failure test harness.
```

Not allowed yet:

```text
Failure-tested highly available Kubernetes deployment.
```
