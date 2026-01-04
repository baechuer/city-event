# Kubernetes Base Configuration

This directory contains the base Kubernetes manifests for the CityEvents platform.

## Directory Structure
```
k8s/
├── base/           # Namespace, Secrets, ConfigMaps
├── infra/          # PostgreSQL, Redis, RabbitMQ
└── apps/           # Microservice deployments
```

## Quick Start

```bash
# 1. Start Minikube
minikube start --driver=docker --cpus=4 --memory=4096 --disk-size=20g
minikube addons enable ingress

# 2. Build and load images
eval $(minikube docker-env)
docker compose build
# OR load images manually
# minikube image load cityevents-auth-service:latest

# 3. Apply manifests (order matters!)
kubectl apply -f k8s/base/
kubectl apply -f k8s/infra/
kubectl apply -f k8s/apps/

# 4. Start tunnel (Windows)
minikube tunnel

# 5. Add to hosts file
# 127.0.0.1 cityevents.local

# 6. Access
# http://cityevents.local
```

## Verify Deployment
```bash
kubectl get all -n city-events
kubectl logs -n city-events deploy/bff-service
```
# 1. Copy and fill secrets
```bash

cp k8s/base/secrets-template.yaml k8s/base/secrets.yaml
```

# 2. Start Minikube & deploy
```bash
minikube start --cpus=4 --memory=4096
minikube addons enable ingress
eval $(minikube docker-env)
docker compose build
kubectl apply -f k8s/base/ -f k8s/infra/ -f k8s/apps/
```