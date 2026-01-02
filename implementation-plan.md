# 城市活动平台 — Implementation Plan (Minikube)

> 目标：先把 React SPA + 独立 BFF + 核心页面跑通，再迁移到 Minikube 做扩容演示与可观测性。
> 原则：边界（/api/*）先行，避免后端 Phase 3 优化返工。


---

## Phase 0 — 对外 API 与数据模型冻结（0.5–1 天）

### Tasks
- 定义 BFF 对外 endpoints（见 docs/architecture.md 第 5 节）
- 统一错误格式（至少：`code`, `message`, `requestId`）
- 明确分页语义（cursor-based，前端采用「加载更多」）
- 决策：是否补 join-service `GET /join/v1/me/participation/{eventID}`（建议做）
- 冻结 Feed Contract（不实现算法，只定接口/数据契约）：
  - 对外：`GET /api/feed`（cursor-based）
  - FeedItem ViewModel：至少包含 event 基础字段 + `score_total`（可选：score_breakdown）
  - 排序稳定性约束：同分时的 tie-breaker（例如 start_time ASC, id ASC）
  - 约定 V0 数据源：可先透传 event-service FTS；未来切换 Redis/Feed Service 不改前端

### Acceptance
- `docs/bff-api.md`（或合并到 architecture）完成
- 关键 ViewModel 列清楚（EventCard / EventView）
### DoD Checklist
- [x] `docs/bff-api.md` 完成：列出所有 `/api/*` endpoints、请求/响应 schema、错误格式、鉴权要求
- [x] Cursor 分页语义明确（字段名、next_cursor/null 行为、排序稳定性、重复/缺页策略）
- [x] 统一错误格式定义完成并示例化（至少 4 类：400/401/429/5xx）
- [x] 关键 ViewModel 定义完成（EventCard、EventView、JoinState）
- [x] 明确 join participation 缺口决策（已做：`GET /join/v1/me/participation` 已实现）并写入文档
- [x] `GET /api/feed` 的请求/响应 schema 与分页语义写入 `docs/bff-api.md`

---

## Phase 1 — 本地开发闭环（Docker Compose）（1–2 天）

### Tasks
- Compose 跑齐：
  - postgres/redis/rabbitmq
  - auth/event/join/email
  - apps/web (React SPA)
  - bff-service (独立 BFF)
  - web-static（可选：Nginx/静态站容器；也可由 bff-service 或专用容器承载静态资源）
- 健康检查 `/healthz` 全部可用
- 统一 Request-ID 头：`X-Request-Id`（生成与透传策略定下来）

### Acceptance
- `docker compose up` 后：可以访问 web 首页
- `curl /api/healthz` 返回 OK
- 任一请求在日志中可用 request-id 串联
### DoD Checklist
- [x] `docker compose up` 一次命令可跑齐：postgres/redis/rabbitmq + auth/event/join/email + bff-service + web
- [x] 每个服务 `/healthz` 返回 200（含 bff-service）
- [x] 从浏览器访问本地 web 成功（静态页或 dev server 均可）
- [x] `curl http://localhost:<bff>/api/healthz` 返回 200
- [x] 任一 BFF 请求日志可看到 `X-Request-Id`，并在下游服务日志出现同一个 ID（证明透传）
- [x] 本地网络隔离策略明确：浏览器是否允许直连微服务（建议：不允许，写进 compose/README）

---

## Phase 2 — React SPA（UI）+ 独立 BFF 骨架（1–2 天）
### Tasks
- 初始化 apps/web：React SPA（推荐 Vite）
- 引入路由：React Router（页面路由）
- 引入 TanStack Query（QueryClient + Provider）
- UI Foundation（必须在写页面前完成）：
  - 安装并配置 Tailwind
  - 引入组件库（建议 shadcn/ui 或 Radix）
  - 建立全局 AppShell（header + container + spacing）
  - 定义基础 UI primitives：
    - Button, Input, Select, Dialog, Toast
    - Skeleton, EmptyState, ErrorState
  - 确定主题与排版（单主题即可，避免后期返工）

- 初始化 bff-service（独立 BFF，轻量 HTTP 服务；推荐 Go/Fastify 二选一）：
  - 对外提供 `/api/*` 入口与统一错误格式（code/message/requestId）
  - 下游调用基建：超时、错误映射、request-id 透传
  - `/api/healthz`
  - 先透传一个读接口：`GET /api/events` → event-service public list


### Acceptance
- `/api/events` 可用（即便只是透传）
- 统一错误结构可在浏览器侧稳定解析
### DoD Checklist
- [x] React SPA 路由可用：`/login`, `/events`, `/events/:id`, `/me/joins`（页面可先占位）
- [x] TanStack Query 接入完成（QueryClientProvider 全局生效）
- [x] UI Foundation 完成且页面只使用 primitives（Button/Input/Toast/Skeleton/Empty/Error）
- [x] bff-service 提供 `/api/healthz` 与统一错误格式（所有错误返回同结构）
- [x] `/api/events` 可工作：SPA → BFF → event-service（抓包/日志可证明链路）
- [x] BFF 对下游的超时与错误映射策略写入 `docs/bff-api.md`（例如 5xx、timeout 统一转 502/504）

---

## Phase 3 — Auth B1 全链路（1–2 天）

### Tasks
- BFF endpoints：
  - `/api/auth/register|login|refresh|logout|me`
- 关键约束：
  - BFF 必须转发 `Set-Cookie(refresh)` 给浏览器
  - access token 只放前端内存（不落 localStorage）
- 前端全局 401 策略：
  - refresh 单飞（并发锁）
  - 成功重放失败请求
  - 失败则清会话并跳转登录
- Dev/Prod 行为一致：
  - 尽量 HTTPS（否则 Secure cookie 行为与 prod 不一致）

### Acceptance
- 刷新页面仍保持登录（通过 refresh 恢复）
- 并发 401 不会触发 refresh 风暴（最多 1 次 refresh）
### DoD Checklist
- [x] `/api/auth/login` 成功后：浏览器收到 refresh cookie（HttpOnly），响应体包含 access token
- [x] 刷新页面后可自动恢复会话（通过 `/api/auth/refresh` → `/api/auth/me`）
- [x] 401 全局处理完成：并发 401 只触发一次 refresh（可用日志计数证明）
- [x] refresh 失败会清理客户端会话并跳转登录（无死循环）
- [x] 429 展示明确提示（而不是无限重试/无提示）
- [x] Dev/Prod cookie 行为差异记录在 `docs/auth.md`（例如 Secure、SameSite、__Host-）

---

## Phase 4 — BFF 核心业务 API（2–4 天）

### Tasks
1) Events feed
- `GET /api/events`（cursor + filters + sort）
- URL 同步 filters/cursor（可分享/可刷新）

2) Event detail view（高价值）
- `GET /api/events/{id}/view`：
  - event detail（event-service）
  - participation（join-service；建议新增 participation-by-event 接口）
  - actions（canJoin/canCancel，BFF 统一算）

3) Join / Cancel
- `POST /api/events/{id}/join`
- `POST /api/events/{id}/cancel`
- 幂等策略：
  - request-id/idem-key 的生成与透传规则定下来（写入文档）
4) Feed API（对外稳定，内部可迭代）
- `GET /api/feed`：
  - V0：透传/复用 event-service 列表（FTS/relevance）
  - V1：从 Redis（或 feed-service）读取预计算结果

### Acceptance
- 详情页不需要前端拼装多个服务响应
- join/cancel 后页面状态一致（Query invalidation）
### DoD Checklist
- [x] `/api/events` 支持 filters + cursor（URL 同步可刷新重现）
- [x] `/api/events/:id/view` 返回一个聚合结构（至少包含 event + joinState + actions）
- [x] joinState 的来源明确：
  - [x] 已实现 `GET /join/v1/me/participation/:eventId`（推荐），或
- [x] `/api/events/:id/join` 与 `/api/events/:id/cancel` 可用
- [x] Mutation 成功后，Query invalidation 策略明确并实现（eventView、meJoins、events 列表）
- [x] 幂等策略落地（请求头/键生成规则）并在文档写清楚
- [x] `/api/feed` 可用（即便 V0 是透传），且 cursor/filters 行为与文档一致

---

## Phase 5 — 前端页面闭环（3–6 天）

### Pages (recommended order)
- `/login`, `/register`
- `/events`（filters + infinite scroll）
- `/events/[id]`（EventView）
- `/me/joins`（先接受 N+1 或简化展示，后续优化）
- organizer/admin 可后置

### Tasks
1. **Type Synchronization**: Align `src/types/api.ts` with BFF `EventViewResponse` and `PaginatedResponse` models.
2. **Auth Refinement**: Ensure `/login` and `/register` have robust loading/error states and 429 handled.
3. **Feed Component Upgrade**:
   - Implement Filter UI (Category, City, Time) synced with URL params.
   - Smooth Infinite Scroll with Skeleton states.
4. **Event Detail Pro Max**:
   - Implement premium `EventView` using the new aggregator response.
   - Intelligent `ActionButtons` using `ActionPolicy` (CanJoin, CanCancel, Reason).
   - Support **Degraded Mode** UI (e.g., banner if participation service is down).
5. **My Joins**: Complete the `/me/joins` page with pagination.
6. **Search & Filter UX**:
   - Implement debounced search in `FilterBar` to prevent excessive URL/API updates.
   - Optimize `EventsFeed` with `placeholderData: keepPreviousData` and smooth transitions.

### UX/Correctness Requirements
- [ ] skeleton/loading/empty/error 状态齐全
- [ ] 搜索防抖（Debounce）与平滑加载（PlaceholderData）
- [x] 429 限流提示明确（不无限转圈）
- [x] join/cancel 防抖 + 可恢复错误提示
- [x] cursor 回退/刷新行为合理

### Acceptance
- 核心旅程跑通：登录 → 浏览 → 详情 → join/cancel → 我的报名
### DoD Checklist
- [x] `/login`：错误、loading、429、成功跳转完整
- [x] `/events`：infinite scroll 正常；filters 改变会重置列表；空/错/加载状态齐全
- [x] `/events/:id`：首屏展示 eventView；join/cancel 按钮状态正确（disabled/spinner）
- [x] `/me/joins`：至少能分页展示；若有 N+1 明确标注 TODO 与后续优化路径
- [x] 所有页面禁止直接调用微服务域名：只打 `/api/*`
- [ ] 所有页面只使用 Phase 2 primitives（禁止散落 CSS 与重复组件）

---

## Phase 6 — User Profile & Security (1–2 天)

Implement user profile management and password security.

### Tasks
1. **Profile Page**:
   - `/profile` path (protected).
   - Display basic user info from `useAuth()` (email, name, role, ID).
   - Premium glassmorphism UI for the profile card.

2. **Change Password**:
   - Implement `ChangePasswordForm` using the BFF `/api/auth/password/change` proxy.
   - Validation: old password, new password (min 12 chars), confirm password.
   - Success: Toast notification and reset form.

3. **Navigation Updates**:
   - Update `NavBar` to make the user profile area clickable or add a dropdown.
   - Link to `/profile`.

### Acceptance
- User can view their profile data accurately.
- User can successfully change their password.
- Navigation to profile is intuitive.

### DoD Checklist
- [ ] `/profile` 页面完成，展示正确的用户信息。
- [ ] 修改密码功能逻辑通畅，且遵循 `auth-service` 的长度要求。
- [ ] 修改密码成功后有明确的 UI 反馈。
- [ ] `NavBar` 中的头像或名称可点击进入 `/profile`。

---

## Phase 7 — Minikube 环境搭建 (2–4 天)

### Tasks
1) Minikube 基础
- 安装/启动：`minikube start`
- 启用 Ingress：`minikube addons enable ingress`
- （可选）metrics-server：`minikube addons enable metrics-server`（给 HPA 用）

2) 部署基础设施（建议 Helm 或 manifests）
- Postgres（PVC）
- Redis
- RabbitMQ（PVC）
- 配置：Secrets/ConfigMaps（DB URL、JWT keys、Rabbit URL 等）

3) 部署应用
- Deployments + Services：auth/event/join/email/web
- readiness/liveness probes（必须）
- 资源 requests/limits（至少给出默认）

4) Ingress
- host：例如 `cityevents.local`（本地 hosts 指向 minikube ip）
- 路由：
  - `/` → web Service（React 静态站点 / 或 web-static 服务）
  - `/api/*` → bff-service Service

- TLS：
  - demo 可先 HTTP，但最终建议 TLS（否则 Secure cookie 行为不一致）
  - Minikube 上可用自签或走本地开发 TLS 方案

### Acceptance
- 外部访问 `http(s)://cityevents.local/` 正常
- 所有 API 只经 `/api/*` 可用
- BFF 对内调用使用 `http://<service-name>` DNS
### DoD Checklist
- [ ] Minikube 启动成功：`minikube status` 正常
- [ ] Ingress addon 启用成功（并能创建 Ingress 资源）
- [ ] 所有 infra（postgres/redis/rabbitmq）在 K8s 内可访问（Service DNS 可解析）
- [ ] 所有应用 Deployment 就绪（readiness 通过；滚动更新不掉服务）
- [ ] Ingress 路由生效：
- [ ] `/` → web（静态站）
- [ ] `/api/*` → bff-service
- [ ] 外部访问 `cityevents.local` 成功（hosts 或 minikube tunnel 方案写入 `docs/minikube.md`）
- [ ] BFF 调用下游使用 `http://<service-name>` DNS（不是 Pod IP；配置可审计）

---

## Phase 7 — 扩容演示（核心展示点）（1–2 天）

### Tasks
- k6 脚本：
- 压 `/api/events`
- 压 `/api/events/{id}/view`
- 演示步骤（写成脚本）
1) event-service replicas=1，跑 k6，记录 p95/错误率
2) `kubectl scale deploy/event-service --replicas=5`
3) 再跑 k6，对比改善
- （可选）HPA 演示：若 metrics-server 可用，可基于 CPU 做自动扩缩

### Acceptance
- `docs/demo-scale.md`：包含命令、预期、输出样例
- 明确论证：BFF 不变，仅扩容 event-service 即改善性能
### DoD Checklist
- [ ] k6 脚本存在且可跑（参数化：host、vus、duration）
- [ ] baseline（event-service=1）跑一次，记录 p95、error rate、RPS
- [ ] scale（event-service=5）跑一次，记录 p95、error rate、RPS
- [ ] `docs/demo-scale.md` 写清楚：命令、预期变化、截图/日志证据
- [ ] 结论明确：BFF 不变，仅扩容 event-service 即改善（数据支撑）

---

## Phase 8 — 可观测性（加分项，建议做）（2–5 天，可并行）

### Tasks
- Trace：
- OTel：bff-service + Go 服务（HTTP server/client）
- Jaeger（或 Tempo）部署

- Metrics：
- Prometheus/Grafana（可最小化）
- RabbitMQ 队列积压
- BFF 下游延迟分布

### Acceptance
- Jaeger 可看到一次请求的跨服务链路
- Grafana 至少有 RPS、p95、错误率
### DoD Checklist
- [ ] Jaeger 可用，能看到一次请求的 trace（至少包含 bff-service → event-service）
- [ ] request-id 与 trace-id 的关联策略明确（日志可串联）
- [ ] Grafana（如启用）至少能看到：RPS、p95、5xx/4xx、RabbitMQ 队列积压（可简化）
- [ ] 观测开关可配置（dev/prod 不同采样率）

---

## Phase 9 — 后端 Phase 3 优化（在前端闭环后做）

### Tasks
- Feed Subsystem（rule-based + 用户 profile + 缓存 + 异步更新）
1) 用户 Profile 与偏好数据源（Auth service 或独立 profile 表）
- category preferences（显式选择 + 行为推断）
- geo preference（home city / latlon）
2) Feed Score 规则实现（确定性加权）
- category +5, geo +3（距离衰减可选）, time +2, popularity +1
- 明确 score_total 与可选 breakdown
3) Redis 结构设计（用于快速分页）
- `feed:user:{uid}` Sorted Set（score + tie-breaker）
- TTL / invalidation 策略
4) Feed Builder Worker（异步预计算与增量更新）
- 触发源（事件）：
- `event.published` / `event.updated` / `event.canceled`
- `join.created` / `join.canceled`（用于偏好与热度）
- `profile.updated`（用户显式偏好变更）
- 幂等：consumer 侧用 event_id 去重
5) BFF 切流
- `/api/feed` 从透传模式切到 Redis/Feed subsystem（对外 contract 不变）
- 回归压测：对比 V0（直查/FTS） vs V1（预计算+缓存）

### Acceptance
- `/api/events` p95 或 DB 压力有可量化下降
- 缓存命中率可观测
### DoD Checklist
- [ ] Feed 评分逻辑在一个地方可审计（SQL 公式或 worker 评分）
- [ ] Redis 缓存策略明确（cache-aside / 预热 / TTL / 失效条件）
- [ ] 回归 k6：对比优化前后 p95/DB 压力（至少一个量化指标）
- [ ] 缓存命中率或“查询次数下降”可观测（日志/metrics 任选其一）
- [ ] Feed Builder Worker 可重放消息不产生重复/乱序问题（幂等可证明）
- [ ] `/api/feed` 在压测下 p95 明显优于 V0（指标写入 demo 文档）

---

## Checklist（你可以逐项勾选）

- [x] Phase 0：BFF endpoints + ViewModel + 错误格式文档完成
- [x] Phase 1：Compose 本地闭环可跑
- [x] Phase 2：React SPA 骨架 + bff-service 骨架 + `/api/events` 可用
- [x] Phase 3：Auth B1 全链路 + refresh 单飞
- [x] Phase 4：`/api/events/{id}/view` + join/cancel
- [x] Phase 5：前端关键页面闭环
- [ ] Phase 6：Minikube 部署 + Ingress 单入口
- [ ] Phase 7：扩容演示脚本 + 指标对比
- [ ] Phase 8：Tracing/Metrics（加分）
- [ ] Phase 9：推荐/缓存优化与回归压测
## Global Quality Gates（全局质量门槛）

- [ ] **单一入口**：浏览器只访问 `<host>/` 与 `<host>/api/*`（禁止直连微服务）
- [ ] **错误一致性**：BFF 对外错误结构稳定（所有 endpoints）
- [ ] **超时与重试可控**：BFF 对下游有统一超时；写操作默认不自动重试
- [ ] **可追踪性**：至少有 request-id 贯穿 BFF→微服务（Phase 1 起就成立）
- [ ] **可扩容性**：所有微服务 Deployment 可独立 scale；BFF 不需改配置或代码
- [ ] **回归验证**：每次引入缓存/算法/观测后，至少跑一次 k6 baseline
