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

## Dependency Failure Evidence

The manual GitHub Actions `Heavy Evidence` workflow can also run:

```bash
bash ./scripts/failure-test-dependencies.sh
```

This command is blocked on the local workstation. In GitHub Actions it starts
the local stack, writes artifacts under `tmp/failure-tests/<run-id>/`, and
checks:

- baseline feed projection before failure scenarios
- Redis stopped while feed reads still fall back to Postgres
- Redis stopped while gateway rate limiting follows configured fail-open behavior
- Redis stopped while bearer-token logout still persists revocation in Postgres
- RabbitMQ stopped while event creation still commits and leaves retryable outbox work
- RabbitMQ restarted while relay and consumers reconnect and the event reaches the feed projection

The RabbitMQ recovery check depends on the relay, feed worker, and notification
worker reconnect loops. If it fails, the correct conclusion is not "HA failed";
it is a concrete broker-recovery bug to fix before stronger resume wording.

## Required Evidence Before Claiming HA

Record the following before using high-availability wording on a resume:

- date and environment
- cluster type and node count
- image versions or commit SHA
- exact command used
- pod deleted and replacement timing
- request behavior during pod deletion
- dependency state for Postgres, RabbitMQ, Redis, and MinIO
- `dependency-failure-evidence` artifact when dependency-failure scenarios are run

## Remaining Failure Tests

Still required for a strong claim:

- API pod deletion while traffic continues
- worker pod deletion during message processing
- repeated RabbitMQ restart evidence under higher message volume
- repeated Redis outage evidence under higher request volume
- Postgres outage and recovery behavior
- concurrent joins above capacity during or after worker failure

The next hardening phase is governed by
`docs/architecture/phase-15-hardening-rubric.md`. A failure scenario does not
count as done unless it records exact commands, observed behavior, and whether
the result changes any resume-safe claim.

## Claim Boundary

Allowed:

```text
Added replicated Kubernetes manifests, PodDisruptionBudgets, guarded pod-failure tooling, RabbitMQ reconnect loops, and GitHub-Actions-only Redis/RabbitMQ failure-evidence tooling.
```

Not allowed yet:

```text
Failure-tested highly available Kubernetes deployment.
```
