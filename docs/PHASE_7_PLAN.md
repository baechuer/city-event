# Phase 7: Kubernetes (Minikube) Implementation Plan

## 1. Environment Setup

### 1.1 Minikube Installation & Configuration
We will use **Minikube** to simulate a local Kubernetes cluster. This provides a realistic environment for testing deployments, services, and ingress rules.

**Prerequisites:**
*   Docker Desktop (already installed on your Windows environment)
*   `kubectl` (Kubernetes CLI)
*   `minikube`

**Setup Steps:**
1.  **Start Cluster**:
    ```bash
    minikube start --driver=docker --cpus=4 --memory=4096 --disk-size=20g
    ```
    *   *Why 4GB RAM?* We are running 5 Go microservices + Postgres + Redis + RabbitMQ. 4GB is a comfortable minimum.

2.  **Enable Addons**:
    ```bash
    minikube addons enable ingress
    minikube addons enable metrics-server # (Optional, for HPA demo later)
    ```

3.  **Tunneling (Windows Specific)**:
    On Windows, Minikube's Ingress IP might not be reachable directly.
    *   Run `minikube tunnel` in a predictable separate terminal (Need to keep it running).
    *   This allocates an IP (usually `127.0.0.1`) to the Ingress Controller LoadBalancer.

### 1.2 Isolation Strategy (Namespaces)
We will use a dedicated **Namespace** to isolate our workload from system components (`kube-system`, `ingress-nginx`).

*   **Namespace Name**: `city-events`
*   **Benefits**:
    *   Clear separation of resources.
    *   Easier cleanup (`kubectl delete ns city-events`).
    *   Access control simulation.

**Action**:
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: city-events
```

---

## 2. Stateful Services (The "Hard" Part)

In Production, we would use managed services (RDS, ElastiCache, Amazon MQ). In Minikube, we must run them inside the cluster.

### 2.1 Strategy: StatefulSets vs Helm
For this Phase, we will use **Standard Kubernetes Manifests (StatefulSets)** rather than Helm charts to keep the learning curve focused on K8s primitives, unless complexity demands otherwise.

| Service | K8s Type | Storage | Persistence Strategy |
|---------|----------|---------|----------------------|
| **PostgreSQL** | `StatefulSet` | `PersistentVolumeClaim` (PVC) | Mount `/var/lib/postgresql/data`. Essential for data survival across pod restarts. |
| **RabbitMQ** | `StatefulSet` | `PersistentVolumeClaim` (PVC) | Mount `/var/lib/rabbitmq`. Needs stable hostname for clustering (even if single node). |
| **Redis** | `Deployment` | `EmptyDir` (or PVC) | For cache, `EmptyDir` is acceptable if we don't care about clearing cache on restart. For reliable Session store, use PVC. We will use **PVC** to match Prod B1 reqs. |

### 2.2 Configuration Management
We will use `ConfigMap` for non-sensitive data and `Secret` for sensitive credentials.

*   **Secrets**: `db-passwords`, `rabbitmq-passwords`, `jwt-secrets`.
*   **ConfigMaps**: `rabbitmq-conf` (plugins), `postgres-init-scripts` (init.sql).

**Key Isolation Note**:
PostgreSQL is currently one container with multiple logical databases (`auth_db`, `event_db`, etc.). In K8s, we will replicate this pattern:
*   **One Postgres Pod** running multiple logical DBs.
*   **Init Container** or ConfigMap script to create these DBs on startup.

---

## 3. Microservices Deployment

### 3.1 Deployment Manifests
Each Go service (`auth`, `event`, `join`, `feed`, `bff`) will have:
1.  **Deployment**:
    *   Replicas: 1 (Scale to >1 for demo).
    *   Image: Local docker image (Need access to local registry).
    *   Env Vars: Sourced from Secrets/ConfigMaps.
    *   **Liveness Probe**: HTTP Get `/healthz` (Restart if dead).
    *   **Readiness Probe**: HTTP Get `/readiness` (Traffic routing).
2.  **Service**:
    *   Type: `ClusterIP` (Internal access only).
    *   BFF accesses downstream via DNS: `http://event-service.city-events.svc.cluster.local:8080`.

### 3.2 Frontend Deployment
*   **Build**: React app built into Nginx container (`nginx:alpine` serving `/dist`).
*   **Config**: Nginx needs `try_files $uri /index.html` for SPA routing.

---

## 4. Ingress (The Entrypoint)

We will verify the "Single Entrypoint" rule using **Ingress Nginx**.

**Rules**:
1.  **Host**: `cityevents.local` (Map to `127.0.0.1` in `C:\Windows\System32\drivers\etc\hosts`).
2.  **Paths**:
    *   `/api` -> `bff-service:80` (Rewrite target might be needed depending on BFF router).
    *   `/` -> `web-service:80`.

---

## 5. Execution Steps (Checklist)

### Step 1: Infra Setup
- [ ] Create Namespace `city-events`.
- [ ] Apply Secrets (convert `.env` to K8s secrets).
- [ ] Deploy Postgres StatefulSet + Service.
- [ ] Deploy RabbitMQ StatefulSet + Service.
- [ ] Deploy Redis Deployment + Service.
- [ ] **Verify**: Shell into Postgres pod and check DB creation.

### Step 2: Build & Load
- [ ] Docker Build all images.
- [ ] `minikube image load <image_name>` (Push local images to Minikube VM).

### Step 3: Deploy Services
- [ ] Apply Deployments for 5 microservices.
- [ ] Check logs: `kubectl logs -n city-events deploy/auth-service`.
- [ ] **Verify**: Database connectivity and RabbitMQ connection successes.

### Step 4: Ingress & Access
- [ ] Apply Ingress resource.
- [ ] Edit Windows Hosts file.
- [ ] Browser test: `http://cityevents.local`.

---

## 6. Testing & Validation
*   **Persistence**: Delete the Postgres pod (`kubectl delete pod postgres-0`). Wait for recreation. Data should persist.
*   **Restart**: Restarts of `event-service` should not cause 500s at the BFF (Readiness probe logic).
*   **Scale**: `kubectl scale deployment event-service --replicas=3`.

