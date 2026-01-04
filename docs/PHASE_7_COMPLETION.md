# Phase 7 Completion: Kubernetes Deployment

This document explains the Kubernetes deployment implementation, strategies used, and how to deploy the CityEvents platform.

---

## 📁 Manifest Structure

```
k8s/
├── README.md                    # Quick start guide
├── base/
│   ├── namespace.yaml          # city-events namespace
│   ├── secrets-template.yaml   # Secrets template (copy & fill)
│   └── configmap-postgres-init.yaml  # DB init script
├── infra/
│   ├── postgres.yaml           # PostgreSQL StatefulSet
│   ├── redis.yaml              # Redis Deployment + PVC
│   └── rabbitmq.yaml           # RabbitMQ StatefulSet
└── apps/
    ├── auth-service.yaml       # Auth microservice
    ├── event-service.yaml      # Event microservice
    ├── join-service.yaml       # Join microservice
    ├── feed-service.yaml       # Feed microservice
    ├── bff-service.yaml        # BFF (API Gateway)
    ├── web.yaml                # React frontend
    └── ingress.yaml            # Nginx Ingress rules
```

---

## 🏗️ Deployment Strategies

### 1. StatefulSet vs Deployment

| Resource Type | Used For | Why |
|---------------|----------|-----|
| **StatefulSet** | PostgreSQL, RabbitMQ | Stable network identity, ordered pod deployment, persistent storage binding |
| **Deployment** | Redis, All microservices | Stateless (or ephemeral state), easy horizontal scaling |

**PostgreSQL as StatefulSet**:
- Ensures `postgres-0` always gets the same PVC
- Ordered startup prevents race conditions
- Headless service provides stable DNS: `postgres-0.postgres.city-events.svc.cluster.local`

### 2. Service Discovery

All services use **Kubernetes DNS** for internal communication:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Ingress (nginx)                          │
│                    cityevents.local:80                          │
└─────────────────────────────────────────────────────────────────┘
                     │                    │
                     ▼                    ▼
              /api/*                    /*
                     │                    │
┌────────────────────┘                    └────────────────────┐
│                                                              │
▼                                                              ▼
┌─────────────────┐                              ┌─────────────────┐
│   bff-service   │                              │       web       │
│   :8080         │                              │       :80       │
└─────────────────┘                              └─────────────────┘
        │
        │ Internal DNS
        ▼
┌─────────────────────────────────────────────────────────────────┐
│  auth-service.city-events.svc.cluster.local:8081               │
│  event-service.city-events.svc.cluster.local:8082              │
│  join-service.city-events.svc.cluster.local:8083               │
│  feed-service.city-events.svc.cluster.local:8084               │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│  postgres.city-events.svc.cluster.local:5432                   │
│  redis.city-events.svc.cluster.local:6379                      │
│  rabbitmq.city-events.svc.cluster.local:5672                   │
└─────────────────────────────────────────────────────────────────┘
```

### 3. Health Probes

Every pod has two probes:

| Probe | Purpose | Action on Failure |
|-------|---------|-------------------|
| **livenessProbe** | Is the process alive? | Restart container |
| **readinessProbe** | Can it accept traffic? | Remove from Service endpoints |

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 10
  periodSeconds: 15

readinessProbe:
  httpGet:
    path: /readyz      # Checks DB connectivity
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 5
```

### 4. Resource Management

Each pod has defined resource requests/limits:

| Service | Memory Request | Memory Limit | CPU Request | CPU Limit |
|---------|----------------|--------------|-------------|-----------|
| Microservices | 64Mi | 256Mi | 50m | 200m |
| PostgreSQL | 256Mi | 512Mi | 250m | 500m |
| RabbitMQ | 256Mi | 512Mi | 100m | 300m |
| Redis | 64Mi | 128Mi | 50m | 100m |
| Web (nginx) | 32Mi | 64Mi | 10m | 50m |

**Total Cluster Resources Required**: ~1.2GB RAM, ~1 CPU core

### 5. Persistence Strategy

| Service | Volume Type | Retention |
|---------|-------------|-----------|
| PostgreSQL | PVC (5Gi) | Data persists across pod restarts |
| RabbitMQ | PVC (1Gi) | Message queues persist |
| Redis | PVC (1Gi) | Session data persists |
| Microservices | None | Stateless - no persistence needed |

---

## 🚀 Deployment Steps

### Step 1: Prerequisites

```bash
# Install Minikube (Windows/Mac/Linux)
# https://minikube.sigs.k8s.io/docs/start/

# Start cluster with adequate resources
minikube start --driver=docker --cpus=4 --memory=4096 --disk-size=20g

# Enable Ingress addon
minikube addons enable ingress
```

### Step 2: Build Images

```bash
# Option A: Build directly in Minikube's Docker
eval $(minikube docker-env)
docker compose build

# Option B: Build locally and load
docker compose build
minikube image load cityevents-auth-service:latest
minikube image load cityevents-event-service:latest
minikube image load cityevents-join-service:latest
minikube image load cityevents-feed-service:latest
minikube image load cityevents-bff-service:latest
minikube image load cityevents-web:latest
```

### Step 3: Create Secrets

```bash
# Copy template and edit with real values
cp k8s/base/secrets-template.yaml k8s/base/secrets.yaml
# Edit secrets.yaml with your credentials

# Apply
kubectl apply -f k8s/base/secrets.yaml
```

### Step 4: Deploy (Order Matters!)

```bash
# 1. Namespace & Config
kubectl apply -f k8s/base/namespace.yaml
kubectl apply -f k8s/base/configmap-postgres-init.yaml

# 2. Infrastructure (wait for each to be ready)
kubectl apply -f k8s/infra/postgres.yaml
kubectl wait --for=condition=ready pod/postgres-0 -n city-events --timeout=120s

kubectl apply -f k8s/infra/redis.yaml
kubectl apply -f k8s/infra/rabbitmq.yaml
kubectl wait --for=condition=ready pod -l app=rabbitmq -n city-events --timeout=120s

# 3. Applications
kubectl apply -f k8s/apps/

# 4. Verify
kubectl get pods -n city-events
```

### Step 5: Configure DNS

**Windows** (`C:\Windows\System32\drivers\etc\hosts`):
```
127.0.0.1 cityevents.local
```

**Mac/Linux** (`/etc/hosts`):
```
127.0.0.1 cityevents.local
```

### Step 6: Start Tunnel (Windows)

```bash
# Keep this running in a separate terminal
minikube tunnel
```

### Step 7: Access

- **Frontend**: http://cityevents.local
- **API**: http://cityevents.local/api/healthz

---

## 🔍 Debugging Commands

```bash
# Check all resources
kubectl get all -n city-events

# View logs
kubectl logs -n city-events deploy/bff-service
kubectl logs -n city-events deploy/auth-service --tail=50

# Shell into pod
kubectl exec -it -n city-events deploy/auth-service -- /bin/sh

# Check events (for startup issues)
kubectl get events -n city-events --sort-by='.lastTimestamp'

# Describe pod (for probe failures)
kubectl describe pod -n city-events -l app=auth-service
```

---

## 📈 Scaling

```bash
# Scale a deployment
kubectl scale deployment event-service -n city-events --replicas=3

# Check pods
kubectl get pods -n city-events -l app=event-service

# Autoscaling (requires metrics-server)
minikube addons enable metrics-server
kubectl autoscale deployment event-service -n city-events --min=1 --max=5 --cpu-percent=70
```

---

## 🧹 Cleanup

```bash
# Delete all resources
kubectl delete namespace city-events

# Stop Minikube
minikube stop

# Delete cluster entirely
minikube delete
```

---

## ⚠️ Production Considerations

1. **Use managed services**: RDS, ElastiCache, Amazon MQ instead of in-cluster StatefulSets
2. **Enable TLS**: Use cert-manager with Let's Encrypt for HTTPS
3. **External secrets**: Use AWS Secrets Manager, Vault, or External Secrets Operator
4. **Monitoring**: Add Prometheus + Grafana for observability
5. **Logging**: Add Fluentd/Fluent Bit to ship logs to ELK/Loki
