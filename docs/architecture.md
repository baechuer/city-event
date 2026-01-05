# 城市活动平台 — Architecture

> 目标：单域名入口、Next.js 作为 UI+BFF、微服务可在 Kubernetes 中独立扩缩容。  
> 演示环境：Minikube（本地 K8s），Ingress Controller = NGINX Ingress。

---

## 1. 系统边界与职责

### 1.1 Edge（入口层）
**NGINX Ingress Controller**
- TLS 终止（HTTPS）
- Host/Path 路由（`/` 与 `/api/*` → Next/BFF）
- 对外负载均衡到多个 Next Pod（若扩容）
- 基础反向代理能力（超时、body size 等）

> 注意：在 K8s 环境中，“最前面的 Nginx”就是 Ingress Controller；通常不再单独运行一个自建 Nginx 容器作为公网入口。

### 1.2 应用层（UI + BFF）
**Next.js (App Router)**
- UI 渲染：SSR/SSG（公开页可用），用户态数据推荐 CSR + TanStack Query
- BFF：Next Route Handlers 提供稳定对外 API：`/api/*`
- 职责：
  - 聚合接口（减少前端瀑布请求）
  - 统一错误格式（`code/message/requestId`）
  - 透传 Request-ID / Trace headers（为后续 OTel 做准备）
- 非职责（保持 lightweight）：
  - 不做服务发现/负载均衡（交给 K8s Service）
  - 不实现推荐/缓存等业务逻辑（交给后端服务 Phase 3）

### 1.3 微服务层（内部服务）
- **auth-service**：JWT 签发/验证、refresh rotation、账户管理
- **event-service**：活动数据事实源，公共列表/过滤/分页
- **join-service**：报名状态机、幂等写入、Outbox 事件发布
- **email-service**：异步通知消费、幂等发送、DLQ 重试

### 1.4 基础设施层
- Postgres：数据存储（events/joins/outbox 等）
- Redis：限流（现状）、后续可扩展为业务缓存（Phase 3）
- RabbitMQ：事件驱动通信（Topic Exchange）、DLQ

---

## 2. 请求线路（Request Flows）

### 2.1 外部入口（浏览器只访问一个 origin）
```
Browser
  |
  v
NGINX Ingress Controller (TLS + Routing + LB)
  |
  v
next-bff Service  ->  next-bff Pods
```

**对外路径约束**
- `https://<host>/`：Next 页面与静态资源
- `https://<host>/api/*`：BFF API（浏览器唯一 API 入口）
- 微服务真实路径（`/auth/v1` `/events/v1` `/join/v1`）不对浏览器暴露（避免绕过 BFF）

### 2.2 内部调用（BFF 到微服务）
```
next-bff Pods
  |
  +--> http://auth-service:8080   (K8s Service -> auth pods)
  +--> http://event-service:8080  (K8s Service -> event pods)
  +--> http://join-service:8080   (K8s Service -> join pods)
  +--> amqp://rabbitmq:5672
  +--> redis://redis:6379
  +--> postgres://postgres:5432
```

> 重点：BFF 永远只调用 **Service DNS**（稳定名），不直接绑定实例地址。扩缩容时无需修改 BFF 代码。

---

## 3. 负载均衡在哪里发生（展示点）

### 3.1 入口负载均衡
- **Ingress Controller** 对 `next-bff` Service 背后的 Pod 做 LB（多副本时）

### 3.2 服务内负载均衡（微服务自主扩容）
- **Kubernetes Service** 对 `event-service` / `join-service` / `auth-service` 的 Pod 做 LB  
- 扩容演示核心：
  - `kubectl scale deploy/event-service --replicas=1->5`
  - BFF 仍然调用 `http://event-service:8080`，无需改动

---

## 4. 认证模型（B1）

### 4.1 Token 存储策略
- **Refresh token**：HttpOnly Cookie（`Path=/`, `Secure`，建议 `__Host-` 前缀）
- **Access token**：前端内存（不落 localStorage），请求时 `Authorization: Bearer <access>`

### 4.2 认证请求线路（浏览器只打 `/api/*`）
- Login：
  - Browser → `POST /api/auth/login` → BFF → auth-service `/auth/v1/login`
  - BFF 透传 `Set-Cookie(refresh)` 回浏览器
  - BFF 返回 access token（JSON），前端存内存
- Refresh（401 或启动恢复）：
  - Browser → `POST /api/auth/refresh`（自动带 refresh cookie）
  - BFF → auth-service `/auth/v1/refresh`（旋转 refresh）
  - BFF 透传 `Set-Cookie` + 返回新 access
- 401 策略：
  - 全局单飞 refresh（并发锁）→ 成功重放 → 失败登出

---

## 5. BFF 对外 API（最小集）

> 这些是**前端唯一依赖**的 API；下游微服务接口变更尽量被 BFF 隔离。

### 5.1 Auth
- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/logout`
- `GET  /api/auth/me`

### 5.2 Events
- `GET /api/events`（cursor + filters + sort）
- `GET /api/events/{id}/view`（聚合 event detail + participation）

### 5.3 Join
- `POST /api/events/{id}/join`
- `POST /api/events/{id}/cancel`
- `GET  /api/me/joins`

**建议的最小后端补强（强烈建议）**
- join-service 增加：`GET /join/v1/me/participation/{eventID}`  
  用于 `/api/events/{id}/view` 直接返回报名状态，避免前端 N+1 或拉大列表匹配。

---

## 6. 目录建议（仓库结构，便于协作）

```
repo/
  services/
    auth-service/
    event-service/
    join-service/
    email-service/
  apps/
    web/                    # Next.js (UI + BFF)
  deploy/
    k8s/
      base/                 # namespace, configmaps, secrets (模板)
      postgres/
      redis/
      rabbitmq/
      services/             # auth/event/join/email deployments+services
      web/                  # next-bff deployment+service
      ingress/              # ingress + tls
  scripts/
    k6/                     # load tests
    minikube/               # enable addons, tunnel, hosts helper
  docs/
    architecture.md
    implementation-plan.md
```

---

## 7. 关键风险与约束（提前规避）

- **Ingress/TLS 与 cookie Secure 行为必须一致**：最终必须 HTTPS，否则 refresh cookie 逻辑会在 prod 才暴雷。
- **App Router 缓存陷阱**：用户态接口在 BFF 层默认 `no-store`，避免跨用户缓存污染。
- **Join 状态查询缺口**：不补 `participation-by-event`，详情页 join 按钮状态会长期别扭。
