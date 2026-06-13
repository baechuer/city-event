# Kubernetes Hardening

## Purpose

This file records the Kubernetes hardening layer and its limits. Kubernetes
manifests can be strong deployment evidence, but they do not prove high
availability until they run in a real or isolated CI cluster with failure and
load evidence.

## Added Controls

| Control | File | Purpose |
| --- | --- | --- |
| Security context patch | `deploy/kubernetes/security-hardening.patch.yaml` | Runs app pods as non-root, drops Linux capabilities, disables privilege escalation, uses RuntimeDefault seccomp, and mounts writable `/tmp` as `emptyDir`. |
| Service account token restriction | `deploy/kubernetes/security-hardening.patch.yaml` | Sets `automountServiceAccountToken: false` for app workloads that do not call the Kubernetes API. |
| Topology spread patch | `deploy/kubernetes/topology-spread.patch.yaml` | Encourages replicas to spread across nodes/zones when available while remaining schedulable in single-node Minikube. |
| HPA manifests | `deploy/kubernetes/hpa.yaml` | Defines CPU-based autoscaling intent for app workloads. Requires a metrics server in the cluster. |
| NetworkPolicies | `deploy/kubernetes/network-policies.yaml` | Adds default deny behavior, same-namespace service communication, DNS egress, and ingress-controller to gateway traffic. |

## Acceptance Rubric

Manifest-ready hardening requires:

- Kustomize renders the base and local overlays.
- App workloads keep readiness/liveness probes, resource requests/limits, and
  at least two replicas.
- Security context, HPA, topology spread, and NetworkPolicy files are included
  in the base Kustomization.
- Docs state that HPA requires metrics-server and NetworkPolicies require a CNI
  that enforces them.

Production evidence requires:

- Multi-node scheduling output proving replicas are spread.
- HPA scale-up/down evidence under load.
- NetworkPolicy connectivity tests for allowed and denied flows.
- Dependency HA or managed dependencies for Postgres, RabbitMQ, Redis, object
  storage, and SMTP.

## Claim Boundary

Safe now:

```text
Added Kubernetes hardening manifests for non-root pods, restricted service account tokens, topology spread, HPA intent, and NetworkPolicies.
```

Not safe yet:

```text
Production Kubernetes hardening is validated.
```
