# High Availability Decision

## Decision

CityEvents is not claiming high availability in the current rebuild.

The selected Phase 11 decision is:

```text
Keep high availability as a documented roadmap limitation until replicas, autoscaling, highly available dependencies, and failure tests are implemented and verified.
```

## Current Evidence

Implemented and verified today:

- Go services are containerized with a shared Dockerfile.
- Kubernetes manifests exist for HTTP services and workers.
- HTTP services expose `/livez` and `/readyz`.
- Kubernetes Deployments define liveness probes, readiness probes, resource requests/limits, and `replicas: 2`.
- PodDisruptionBudgets preserve at least one pod for each workload during voluntary disruption.
- ConfigMaps and Secret templates separate configuration from code.
- Ingress manifests declare TLS routing, forced HTTPS redirect, and a cert-manager certificate example.
- Phase 10, Phase 13, and Phase 14 manifest validation pass through `scripts/verify-phase-10.sh`, `scripts/verify-phase-13.sh`, and `scripts/verify-phase-14.sh`.
- A Kubernetes overlay and GitHub-Actions-only Minikube smoke runner exist for deploy-and-pod-replacement evidence.

This evidence supports:

```text
Kubernetes-ready microservices with replicated stateless workloads.
```

It does not support:

```text
Highly available Kubernetes deployment.
```

## Why HA Is Deferred

The current manifests use two replicas per service and worker, plus PodDisruptionBudgets. This is useful deployment readiness evidence, but it is not the same as high availability.

The backing services are also single-instance in the current local stack:

- Postgres
- RabbitMQ
- Redis
- MinIO
- Mailpit

If any of those dependencies fail, additional stateless service replicas do not make the full system highly available. The new `replicas: 2` and PodDisruptionBudgets improve pod replacement and voluntary disruption readiness, but claiming HA still requires dependency HA, autoscaling evidence, traffic-level validation, and recorded live failure tests.

The Kubernetes overlay can create single-instance backing services for Minikube smoke testing in GitHub Actions. That improves evidence beyond static manifests, but it is still not production HA because the cluster and backing dependencies are not highly available.

Current note from 2026-06-11: local workstation execution is blocked after a
stability warning. The first live Minikube smoke did not reach app deployment
because Minikube failed with `K8S_APISERVER_MISSING`. Later attempts showed this
is heavy host-level evidence, so future accepted runs must use the manual
GitHub Actions `Heavy Evidence` workflow and uploaded artifacts.

TLS is also not production-proven yet. The manifests identify the expected `cityevents-tls` secret and include a cert-manager `Certificate` example, but a real cluster still needs a valid certificate issuer, DNS, and an ingress smoke test.

## What Would Make HA True

To claim high availability, the project still needs all of the following:

- live verification that multiple replicas continue serving traffic during pod failure
- rolling update settings
- Horizontal Pod Autoscaler or a documented manual scaling policy
- managed or replicated Postgres
- RabbitMQ quorum queues or a RabbitMQ cluster
- managed Redis, Redis Sentinel, or Redis Cluster
- durable persistent volume and backup strategy
- failure tests that prove recovery behavior

## Required Failure Tests Before Claiming HA

Minimum tests:

- delete one API pod during health traffic and verify requests continue through another ready pod
- delete one worker during message processing and verify RabbitMQ redelivery plus idempotent handling
- restart RabbitMQ during outbox publishing and verify outbox replay recovers unsent messages
- restart Redis during feed reads and verify Postgres fallback prevents corrupt state
- run concurrent joins above event capacity and verify no overbooking
- record exact commands, dates, environment, and observed results

The repo now includes a guarded starter script:

```bash
bash ./scripts/failure-test-kubernetes.sh
bash ./scripts/failure-test-kubernetes.sh --live --deployment api-gateway
bash ./scripts/k8s-live-smoke.sh --start-minikube --run-failure
```

The first command is static and local-safe. The second and third commands are
blocked outside GitHub Actions and only count as evidence when run by the manual
`Heavy Evidence` workflow with uploaded artifacts reviewed.

## Safe Resume Wording

Use:

```text
Prepared Go microservices for Kubernetes deployment with health probes, configuration separation, resource limits, two-replica workload manifests, PodDisruptionBudgets, and documented high-availability requirements.
```

Use:

```text
Designed the deployment path for high availability by identifying required replicas, autoscaling, highly available dependencies, and failure tests.
```

Do not use yet:

```text
Deployed a highly available Kubernetes system.
```

Do not use yet:

```text
Implemented autoscaled production microservices.
```

## Interview Explanation

Kubernetes readiness and high availability are different claims.

This project currently proves that the services can be containerized and described with Kubernetes manifests. It also proves important reliability mechanisms at the application layer: transactional outbox, persistent RabbitMQ messages, publisher confirms, idempotent consumers, Redis fallback, and correlation IDs.

The project does not yet prove production high availability because the deployment has not been live-tested under failure and the backing dependencies are not highly available. The honest next step is to run the manual GitHub Actions heavy-evidence workflow, make dependencies HA, add autoscaling or a scaling policy, then record pod and dependency failure tests.
