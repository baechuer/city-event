# Kubernetes Readiness

These manifests prepare CityEvents for Kubernetes deployment. They do not prove high availability.

The current HA decision is documented in `../../docs/architecture/high-availability-decision.md`.

## Build Images

Build one image per command using the shared Dockerfile:

```bash
docker build --build-arg SERVICE=api-gateway -t cityevents/api-gateway:dev .
docker build --build-arg SERVICE=auth-service -t cityevents/auth-service:dev .
docker build --build-arg SERVICE=event-registration-service -t cityevents/event-registration-service:dev .
docker build --build-arg SERVICE=feed-service -t cityevents/feed-service:dev .
docker build --build-arg SERVICE=notification-service -t cityevents/notification-service:dev .
docker build --build-arg SERVICE=media-service -t cityevents/media-service:dev .
docker build --build-arg SERVICE=feed-worker -t cityevents/feed-worker:dev .
docker build --build-arg SERVICE=notification-worker -t cityevents/notification-worker:dev .
docker build --build-arg SERVICE=media-worker -t cityevents/media-worker:dev .
docker build --build-arg SERVICE=outbox-relay -t cityevents/outbox-relay:dev .
```

## Apply Manifests

Replace `secret.example.yaml` values before any real deployment. The example contains a seed admin so the role workflow can be verified, but the password is not production-safe.

Create a TLS secret named `cityevents-tls` in the `cityevents` namespace before relying on ingress TLS. For a local `cityevents.local` host, create this secret manually:

```bash
kubectl create secret tls cityevents-tls \
  --namespace cityevents \
  --cert path/to/tls.crt \
  --key path/to/tls.key
```

For a real public domain, install cert-manager, create a production `ClusterIssuer`, update `cert-manager-certificate.example.yaml` with the real DNS name, and apply it:

```bash
kubectl apply -f deploy/kubernetes/cert-manager-certificate.example.yaml
```

Cert-manager then creates and renews the same `cityevents-tls` secret that `ingress.yaml` references.

```bash
kubectl apply -k deploy/kubernetes
```

## Ingress

`ingress.yaml` routes `cityevents.local/v1`, `/readyz`, `/livez`, and `/metrics` to the API gateway. It assumes an ingress controller that supports `ingressClassName: nginx`. The manifest declares TLS for `cityevents.local`, references the `cityevents-tls` secret, and enables NGINX SSL redirect annotations.

If you switch to a public hostname, update `ingress.yaml`, `configmap.yaml` `CORS_ALLOWED_ORIGINS`, and `cert-manager-certificate.example.yaml` together.

Ingress is not high availability by itself. It only exposes HTTP routing. HA still needs more than one gateway pod, pod disruption budgets, autoscaling, multi-node scheduling, and highly available data stores.

## Claim Boundary

Allowed:

```text
Kubernetes-ready service manifests with health probes, resource limits, configuration separation, forced HTTPS ingress routing, and a cert-manager certificate example.
```

Not allowed yet:

```text
Highly available Kubernetes deployment.
```

High availability still requires multiple replicas, autoscaling, highly available Postgres/RabbitMQ/Redis, and failure testing.
