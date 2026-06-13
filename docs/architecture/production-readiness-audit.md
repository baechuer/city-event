# Production Readiness Audit

## Verdict

CityEvents is not production-ready yet.

It is deployable as a controlled demo, local stack, or staging candidate. The
latest `v2-rebuild` CI run is green, the services build into containers, and
the repository has meaningful unit, integration, browser E2E, Kubernetes
manifest, and failure-evidence tooling. That is strong portfolio evidence.

It is not ready for a public production deployment because the live Kubernetes,
load, and dependency-failure evidence has not been run, production secrets and
managed dependencies are not provisioned, Kubernetes hardening is incomplete,
and there is no release/deployment pipeline to an actual cluster.

## External Baseline

This audit uses current public guidance as the bar:

- OWASP Application Security Verification Standard:
  https://owasp.org/www-project-application-security-verification-standard/
- OWASP Cheat Sheet Series:
  https://cheatsheetseries.owasp.org/
- Kubernetes Security Checklist:
  https://kubernetes.io/docs/concepts/security/security-checklist/
- Kubernetes production cluster guidance:
  https://kubernetes.io/docs/setup/production-environment/
- OpenTelemetry observability concepts:
  https://opentelemetry.io/docs/concepts/observability-primer/

## What Is Already Strong

| Area | Current evidence |
| --- | --- |
| CI | Latest `v2-rebuild` push run `27347861283` succeeded for Go tests, frontend unit tests, phase verification, browser E2E, and container builds. |
| Backend separation | Go services are separated under `cmd/` and `internal/services/`; shared platform code lives under `internal/platform/`. |
| Auth security | Short-lived access tokens, memory-only frontend access token handling, rotating HttpOnly refresh cookies, CSRF protection for refresh/logout, bcrypt password hashing, JWT/RBAC gateway boundary. |
| Event consistency | Event registration owns capacity, waitlist, cancellation, promotion, idempotency, row-level locking, and transactional outbox writes. |
| Messaging reliability | RabbitMQ publisher confirms, persistent messages, increasing-delay outbox retries with terminal `DEAD` rows, bounded consumer retry queues with DLQs, idempotent consumers, and reconnecting relay/workers. |
| Redis use | Redis is used as a cache/accelerator, not source of truth; feed and revocation fallback behavior is tested. |
| Rate limiting | Shared Redis-backed fixed-window limiter exists with explicit fail-open/fail-closed behavior and tests. |
| Kubernetes readiness | Deployments, Services, ConfigMap, Secret template, Ingress, readiness/liveness probes, resource requests/limits, two replicas, PodDisruptionBudgets, security-context patches, HPA intent, topology spread, and NetworkPolicies exist. |
| Browser path | Playwright covers organizer publish and attendee join against the local stack. |
| Evidence discipline | Heavy evidence scripts are blocked locally and intended for isolated GitHub Actions runners. |

## Production Blockers

| Priority | Blocker | Why it matters | Correction |
| --- | --- | --- | --- |
| P0 | Heavy evidence has not run | CI is green, but Kubernetes live smoke, load matrix, and dependency-failure artifacts are still pending. | Make `heavy-evidence.yml` active from the default branch or dispatchable ref, run it, review artifacts, and record results. |
| P0 | No real production environment | The repo has manifests, not a verified cluster, domain, TLS certificate, database, broker, cache, object store, or SMTP provider. | Choose a target environment and provision managed Postgres, RabbitMQ or equivalent broker, Redis, object storage, DNS, ingress, and TLS. |
| P0 | Secrets are examples/dev defaults | `secret.example.yaml` is a placeholder and config defaults include local credentials. | Use External Secrets, Sealed Secrets, or a cloud secret manager; remove seeded admin defaults from production paths; rotate all real credentials. |
| P0 | No release pipeline | Images are built in CI but not pushed with immutable tags and not deployed. | Add GHCR/ECR push, SBOM, vulnerability scan, signed image tags, environment promotion, and controlled deploy job. |
| P0 | Kubernetes security hardening is manifest-ready, not live-proven | Manifests now define NetworkPolicies, pod/container security contexts, service account restrictions, HPA, and topology spread, but they have not been validated in a real multi-node cluster. | Run CI/staging cluster evidence for scheduling, NetworkPolicy allow/deny flows, HPA behavior, ingress controls, and production namespaces. |
| P0 | Dependency HA is not implemented | App replicas do not make the system highly available if Postgres, RabbitMQ, Redis, or object storage are single points of failure. | Use managed/replicated Postgres, RabbitMQ quorum queues or managed broker, managed Redis/cluster, durable object storage, and backup/restore drills. |
| P1 | Observability is manifest-ready, not production-grade | Metrics/logs/tracing are useful and the repo now has collector, alert, and dashboard config, but there is no live backend, SLO, alert delivery test, or central log retention. | Add OpenTelemetry SDK spans/exporter, deploy Collector/Prometheus/Grafana/Alertmanager, add log aggregation, and record live alert evidence. |
| P1 | Security testing is incomplete | Existing auth/RBAC/CSRF tests and CI security gates are useful, but there is no full ASVS checklist, container scanning after image push, secret scanning proof, or DAST baseline. | Add secret scanning, image scanning, OWASP ZAP baseline, ASVS checklist, and production secret manager evidence. |
| P1 | Database lifecycle is incomplete | Migrations are local/scripted; production needs rollout/rollback and backup safety. | Add Kubernetes migration Job, migration locking, backup policy, restore test, and rollback plan. |
| P1 | DLQ operations are partial | DLQs, terminal outbox rows, read-only inspection, guarded replay/requeue scripts, alert rules, and dashboard config exist, but production still needs live alert delivery and reviewed replay evidence. | Deploy alerts/dashboard, add operator approval flow, and record replay drills on an isolated runner. |
| P1 | Frontend deployment is not represented in Kubernetes | API ingress exists, but a production frontend host/CDN/deployment path is not defined. | Decide frontend hosting: static CDN, object storage + CDN, or Kubernetes web deployment; add CSP/security headers. |
| P1 | Admin/audit controls need hardening | Roles exist, but privileged actions need stronger audit trails and operational controls. | Add immutable audit records for role changes, event cancellation, admin dejoin/cancel actions, and production admin bootstrap procedure. |

## Resume-Safe Framing

Safe now:

```text
Built a Go microservices event platform with JWT/RBAC auth, transactional event registration, RabbitMQ outbox messaging, Redis-backed caching/rate limiting, browser E2E tests, CI gates, and Kubernetes-ready manifests.
```

Not safe yet:

```text
Production deployed, highly available, high-throughput, or exactly-once message consumption.
```

Allowed after heavy evidence passes:

```text
Verified Kubernetes smoke/load/failure evidence in GitHub Actions for a staging-style environment.
```

Still not allowed after that alone:

```text
Production high availability.
```

## Correction Plan

### Phase A: Make Evidence Run

1. Put `heavy-evidence.yml` on the default branch or otherwise make it
   dispatchable in GitHub Actions.
2. Run Kubernetes live smoke, load matrix, and dependency-failure evidence.
3. Archive the run URL, commit SHA, inputs, and artifact summaries.
4. Fix any failing scenario before changing resume wording.

### Phase B: Make Deployment Real

1. Pick a target: managed Kubernetes or simpler PaaS/container service.
2. Create production/staging overlays separate from local/minikube overlays.
3. Push immutable images to a registry.
4. Provision secrets through a secret manager.
5. Create a migration Job and rollback path.
6. Deploy frontend through CDN/static hosting or a hardened web deployment.

### Phase C: Harden Security

1. Add Kubernetes NetworkPolicies.
2. Add pod/container security contexts and read-only root filesystems where
   feasible.
3. Disable service account token mounting unless a pod needs Kubernetes API
   access.
4. Add SAST, dependency scanning, image scanning, secret scanning, and DAST.
5. Add CSP/HSTS/security headers for the frontend/API ingress.
6. Add admin audit logging.

### Phase D: Prove Reliability

1. Use highly available managed dependencies.
2. Add HPA and resource tuning from measured load.
3. Run load tests under stated targets.
4. Run pod, broker, cache, database, and object-store failure drills.
5. Prove backup restore.
6. Add SLOs, alerts, dashboards, and runbooks.

## Final Assessment

The project is strong enough to present as a serious backend/platform portfolio
project if the wording stays evidence-bounded. It is not production-ready. The
next useful engineering work is not another feature; it is making the deployment
target real, running the isolated heavy evidence, and closing the production
security/operations gaps above.
