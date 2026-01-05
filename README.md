# CityEvents 🎉

A production-grade event management platform built with **Go microservices**, **React**, and **Kubernetes**.

![Architecture](https://img.shields.io/badge/Architecture-Microservices-blue)
![Backend](https://img.shields.io/badge/Backend-Go-00ADD8)
![Frontend](https://img.shields.io/badge/Frontend-React%20%2B%20TypeScript-61DAFB)
![Infrastructure](https://img.shields.io/badge/Infra-Kubernetes-326CE5)

## 🎯 Overview

CityEvents enables users to discover, create, and join local meetups and events. The platform demonstrates enterprise-grade patterns including:

- **Event-driven architecture** with RabbitMQ
- **CQRS-style read optimization** via feed service
- **Concurrent capacity management** with idempotent joins
- **Real-time observability** with Prometheus + Grafana

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Ingress (NGINX)                          │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │   BFF Service   │  (API Gateway, Auth, Aggregation)
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ Auth Service │    │Event Service │    │ Join Service │
│   (JWT)      │    │   (CRUD)     │    │  (Capacity)  │
└──────────────┘    └──────────────┘    └──────────────┘
        │                    │                    │
        └────────────────────┼────────────────────┘
                             │
                    ┌────────▼────────┐
                    │    RabbitMQ     │  (Event Bus)
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        ▼                    ▼                    ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ Feed Service │    │Email Service │    │Media Service │
│ (Read Model) │    │(Notifications│    │  (Images)    │
└──────────────┘    └──────────────┘    └──────────────┘
```

## ✨ Key Features

| Feature | Implementation |
|---------|---------------|
| 🔐 **Authentication** | JWT + Refresh tokens, OAuth (Google/GitHub) |
| 📅 **Event Management** | CRUD, Publishing workflow, Moderation |
| 👥 **Join System** | Capacity limits, Waitlist, Idempotent requests |
| 🔍 **Smart Feed** | Category filters, Search, Personalization |
| 📧 **Notifications** | Email via async consumers (RabbitMQ) |
| 📊 **Observability** | Prometheus metrics, Grafana dashboards |
| 🖼️ **Media Upload** | Image processing, Cropping, CDN-ready |

## 🛠️ Tech Stack

### Backend
- **Language**: Go 1.23
- **Database**: PostgreSQL 15
- **Cache**: Redis 7
- **Message Queue**: RabbitMQ 3.12
- **Object Storage**: MinIO (S3-compatible)

### Frontend
- **Framework**: React 18 + TypeScript
- **Styling**: Tailwind CSS
- **State**: TanStack Query
- **Build**: Vite

### Infrastructure
- **Container Orchestration**: Kubernetes (Minikube for local)
- **Ingress**: NGINX Ingress Controller
- **Monitoring**: Prometheus + Grafana
- **CI/CD**: GitHub Actions

## 🚀 Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.23+
- Node.js 22+
- (Optional) Minikube for K8s deployment

### Local Development (Docker Compose)

```bash
# Clone repository
git clone https://github.com/baechuer/real-time-ressys.git
cd real-time-ressys

# Start infrastructure
docker compose -f compose.infra.yml up -d

# Start all services
docker compose up -d --build

# Access the app
open http://localhost:3000
```

### Kubernetes Deployment

```bash
# Start Minikube
minikube start --memory=8192 --cpus=4

# Deploy infrastructure
kubectl apply -f k8s/base/
kubectl apply -f k8s/infra/

# Deploy applications
kubectl apply -f k8s/apps/

# Access via port-forward
kubectl port-forward svc/ingress-nginx-controller -n ingress-nginx 8080:80
open http://localhost:8080
```

## 📁 Project Structure

```
├── apps/
│   └── web/                 # React frontend
├── services/
│   ├── auth-service/        # Authentication & users
│   ├── event-service/       # Event CRUD & publishing
│   ├── join-service/        # Participation management
│   ├── feed-service/        # Read-optimized queries
│   ├── email-service/       # Notification delivery
│   ├── media-service/       # Image upload & processing
│   └── bff-service/         # API gateway
├── k8s/
│   ├── base/               # Namespace, secrets
│   ├── infra/              # Postgres, Redis, RabbitMQ
│   └── apps/               # Service deployments
└── tests/
    └── load/               # k6 load tests
```

## 🧪 Testing

```bash
# Unit tests (all services)
go test ./services/...

# Load test (1500 concurrent join requests)
k6 run tests/load/join-event.js
```

### Load Test Results
- ✅ **0% failure rate** under 50 req/s sustained load
- ✅ **100 successful joins** before capacity reached
- ✅ **P95 latency**: 1.8s (single replica)

## 📊 Observability

Access Grafana dashboards:
```bash
kubectl port-forward svc/grafana -n city-events 3000:3000
```

Available dashboards:
- **Business Metrics**: Joins/sec, Logins, Event creations
- **System Health**: Request rate, Error rate, Latency (RED)

## 🔒 Security Features

- JWT with refresh token rotation
- Rate limiting (Redis-backed sliding window)
- RBAC for admin/moderator actions
- Input validation & sanitization
- Secure cookie handling

## 📝 API Overview

| Endpoint | Description |
|----------|-------------|
| `POST /api/auth/login` | User authentication |
| `GET /api/events` | List events (paginated) |
| `POST /api/events/{id}/join` | Join an event |
| `GET /api/feed/recommended` | Personalized feed |

## 🤝 Contributing

This is a portfolio project. Feel free to explore and learn from it!

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

**Built with ❤️ as a demonstration of production-grade architecture patterns.**
