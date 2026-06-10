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
- Kubernetes Deployments define liveness probes, readiness probes, and resource requests/limits.
- ConfigMaps and Secret templates separate configuration from code.
- Phase 10 manifest validation passes through `scripts/verify-phase-10.sh`.

This evidence supports:

```text
Kubernetes-ready microservices.
```

It does not support:

```text
Highly available Kubernetes deployment.
```

## Why HA Is Deferred

The current manifests use one replica per service. Kubernetes can restart a failed single pod, but restart behavior is not the same as continuous availability during failure.

The backing services are also single-instance in the current local stack:

- Postgres
- RabbitMQ
- Redis
- MinIO
- Mailpit

If any of those dependencies fail, additional stateless service replicas would not make the full system highly available. Adding `replicas: 2` without dependency HA, autoscaling evidence, and failure tests would overstate what the project proves.

The local Kubernetes client dry-run is also not fully verified in this environment because the local kubeconfig is not readable. `kubectl kustomize` validation is available, but a real pod-failure test needs a working local cluster.

## What Would Make HA True

To claim high availability, the project needs all of the following:

- multiple replicas for stateless HTTP services
- readiness probes that remove unhealthy pods from traffic
- PodDisruptionBudgets for replicated services
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

## Safe Resume Wording

Use:

```text
Prepared Go microservices for Kubernetes deployment with health probes, configuration separation, resource limits, and documented high-availability requirements.
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

The project does not yet prove production high availability because the deployment is not running with tested replicas and the backing dependencies are not highly available. The honest next step is to run the manifests in a local or cloud cluster, add replicas and disruption budgets where safe, make dependencies HA, then run pod and dependency failure tests.
