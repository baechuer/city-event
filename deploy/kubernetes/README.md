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

Replace `secret.example.yaml` values before any real deployment.

```bash
kubectl apply -k deploy/kubernetes
```

## Claim Boundary

Allowed:

```text
Kubernetes-ready service manifests with health probes, resource limits, and configuration separation.
```

Not allowed yet:

```text
Highly available Kubernetes deployment.
```

High availability still requires multiple replicas, autoscaling, highly available Postgres/RabbitMQ/Redis, and failure testing.
