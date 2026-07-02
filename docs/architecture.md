# 架构与分层约定

本文描述本仓库作为**通用脚手架**时的分层方式。新业务按同一套路扩展，避免 handler 直连 SQL、避免跨层泄漏。

## 请求链路

```text
Client
  → internal/http（Router + Middleware + Handler）
  → internal/service（业务用例）
  → internal/repo（数据访问接口）
  → internal/repo/pg + sqlcgen（PostgreSQL）
  → Redis / 外部服务（在 service 或 store 层使用）
```

装配发生在 `internal/app/app.go`：创建连接 → `pg.NewStore` → 构造各 `Service` → 填入 `http.Deps` → `http.NewRouter`。

## 目录职责

### `internal/http`

- **只做**：参数绑定、调用 Service、用 `pkg/resp` 返回 JSON。
- **不做**：SQL、复杂业务判断、事务编排。
- 新模块：新增 `xxx.go`，提供 `registerXxxRoutes(group, deps)`，在 `router.go` 挂载。

路由分组建议：

| 前缀 | 鉴权 | 用途 |
|------|------|------|
| `/api/v1/auth/*` | 无 | 登录、注册、验证码 |
| `/api/v1/user/*` | JWT | 面向终端用户 |
| `/api/v1/admin/*` | JWT + `RequireRole("admin")` | 面向运营/管理 |

管理端统一挂在 `router.go` 的 `admin := v1.Group("/admin", Auth, UserRateLimit, RequireRole("admin"))`，真实业务只需要继续注册到该分组。

### `internal/service/{module}`

- 定义 `Service` 接口与实现。
- 依赖 `repo` 接口，不依赖 `sqlcgen` 包名（通过 `pg` 仓库实现转换）。
- 返回领域错误或 `pkg/errcode.Error`。

### `internal/repo`

- `types.go`：领域实体（与 DB 解耦的 struct）。
- `user_repo.go` 等：Repository **接口**。
- `pg/`：PostgreSQL 实现，内部使用 `sqlcgen.Queries`。
- `sqlc/query/*.sql`：手写 SQL，`-- name: Xxx` 注解。

### `pkg/`

- 与业务无关、可被多个项目复制的库：`resp`、`errcode`、`validator`、`request`、`i18n`。

### `internal/auth`

- JWT 签发/解析、OAuth 策略、密码与验证码**工具**（非 HTTP）。
- 认证**流程**在 `internal/service/auth`。

## 新增业务模块（标准步骤）

以模块名 `order` 为例：

### 1. 数据库

```bash
# 新建 migration
backend/migrations/000002_orders.up.sql
backend/migrations/000002_orders.down.sql
```

执行 `migrate up` 后，把新文件加入 `backend/sqlc.yaml` 的 `schema` 列表。

### 2. sqlc

在 `internal/repo/sqlc/query/orders.sql` 编写查询，然后：

```bash
cd backend && sqlc generate
```

### 3. Repo

- `internal/repo/order_repo.go` — 接口
- `internal/repo/pg/order_repo.go` — 实现
- 在 `pg/store.go` 暴露 `Orders() repo.OrderRepo`

### 4. Service

- `internal/service/order/service.go` — 接口 + 实现
- 在 `internal/app/app.go` 中 `orderservice.New(pgStore.Orders())` 并注入 `http.Deps`

### 5. HTTP

- `internal/http/order.go` — Handler + `registerOrderRoutes`
- `router.go` 中挂到 `/api/v1/user` 或 `/api/v1/admin`

### 6. 文档（可选）

- `docs/api/order.md` 或 OpenAPI

## 配置

加载顺序：`.env` → `config/base.yaml` → `config/{APP_ENV}.yaml` → `config/local.yaml`（gitignore）→ 环境变量。

`.env` 只用于本地开发，`gotenv.Load` 不会覆盖系统环境变量。生产环境建议直接使用系统环境变量或部署平台 Secret。

常用变量见 `backend/.env.example`。

以下配置已接入运行时：

- PostgreSQL pool：`database.pool.*`
- Redis pool：`redis.pool_size`
- Redis key 前缀：`redis.key_prefix`
- 日志：`log.level`、`log.format`（字段规范见 [logging.md](logging.md)）
- 限流：`rate_limit.per_minute`（IP）、`rate_limit.user_per_minute`（已鉴权 user_id）
- CORS：`cors.allow_origins`、`cors.allow_credentials`
- Auth 固定验证码：`auth.mock_code_enabled`
- 多/单设备 Session：`auth.multi_device_enabled`（详见 [session.md](session.md)）
- OAuth：`oauth.dev_mode`、`oauth.providers`（详见 [oauth.md](oauth.md)）

启动时会执行安全校验：生产环境禁止默认 `JWT_SECRET`，禁止启用固定验证码/OAuth 开发模式，且必须显式配置 CORS 来源。

## 登出与会话

会话由 `internal/session` 管理，JWT 携带 `sid`（session_id）。多/单设备行为由 `auth.multi_device_enabled` 控制，完整说明见 [session.md](session.md)。

### 登录

每次登录（含 OAuth）生成新的 `session_id`，写入 access / refresh token 的 `sid` 声明，并在 Redis 注册会话。

| 模式 | 新登录行为 |
|------|------------|
| 多设备（`multi_device_enabled=true`，默认） | 新建 session，各设备互不影响 |
| 单设备（`false`） | 新登录前 `RevokeAll` 踢掉旧会话 |

### 刷新

`POST /api/v1/auth/refresh` 轮换 refresh token 时**保持同一 `sid`**，不新建 session。旧 refresh token 写入黑名单。

### 登出

`POST /api/v1/auth/logout` 需提交 `refresh_token`，服务端会：

1. 将 refresh token 写入 Redis 黑名单（后续 Refresh 拒绝）。
2. 吊销当前 `sid` 对应 session（**仅当前设备**；多设备下其他设备不受影响）。

### 全端吊销（改密 / 禁用）

改密（`PATCH /api/v1/user/password`）或后台禁用用户时：

1. `RevokeAll` 吊销该用户全部 session。
2. 写入用户级 `revoke:uid:{user_id}` 时间戳，`Auth` 中间件比对 access token 的 `iat`。

普通登出**不再**写 `revoke:uid`，避免多设备模式下误伤其他设备。

### 中间件校验

`Auth` 中间件依次检查：

1. JWT 签名与有效期
2. 用户级 `revoke:uid`（改密/禁用兜底）
3. `sid` 在 Redis 中仍有效

### 行为约定

- **登出失败即报错**：refresh 黑名单或 session 吊销写入失败时返回 `ErrInternal`。
- **吊销检查 fail-open**：Redis 宕机时 `revoke:uid` / session 检查放行并打 Warn 日志，避免缓存故障拖垮全站鉴权；极高安全需求可自行改为 fail-closed。

Redis 键说明见 [redis-keys.md](redis-keys.md) 第 2、7 节。

## OAuth

生产环境通过 `internal/oauth` 校验各平台 token（微信 / Apple / Google），开发模式（`oauth.dev_mode=true`）允许仅传 `open_id` 跳过远程校验。

```json
{
  "grant_type": "oauth",
  "provider": "wechat",
  "open_id": "wx_openid_xxx",
  "access_token": "..."
}
```

流程：按 `provider` 校验 token → 用 `open_id` 查 `user_oauth` → 已绑定则登录 → 未绑定则自动建用户并写入绑定记录。

详见 [oauth.md](oauth.md)。生产环境须关闭 `oauth.dev_mode` 并配置各平台 `client_id` / `app_id`。

## 中间件顺序

`RequestID` → `I18n` → `Zap` → `Recovery` → `Safe` → `RateLimit(IP)` → `CORS` → 路由级 `Auth` → `RateLimit(user_id)` → `RequireRole`。

## 健康检查

- `/health`：轻量存活检查，只说明进程可响应。适合 **liveness** 探针。
- `/ready`：就绪检查，会 ping PostgreSQL 与 Redis，适合 **readiness** 探针。K8s 示例见 [deploy.md](deploy.md) 第四节与 `deploy/k8s/`。

## 工程化命令

根目录提供 `Makefile`：

```bash
make compose-up
make migrate-up
make run
make test
make build
make sqlc
```

CI 位于 `.github/workflows/ci.yml`，会校验 sqlc 生成代码、**OpenAPI 契约**、运行测试并构建后端。带 `-tags=integration` 的集成/E2E 测试需本机 Docker（`make test-integration`、`make test-e2e`）。

## 多端与子服务

- **frontend/**：只调 `backend` HTTP API。
- **apps/node/**：BFF、WebSocket；对前端暴露，可再调 backend。
- **apps/python/**：算力密集任务；由 backend 通过 HTTP/RPC 调用，对前端透明。

克隆仓库后，子服务目录可按需创建，不必一次写满。

## 内置示例：Todo 模块

仓库自带可运行的 **Todo CRUD**（`internal/service/todo`），走完整分层链路。用于：

- 克隆后对照改出自己的第一个业务模块
- 验证迁移、sqlc、登录鉴权是否正常

接口说明与 curl 示例见 [examples/todo-module.md](examples/todo-module.md)。上线真实产品前可整段删除示例代码与 `000002` 迁移。

## 与具体项目的关系

本仓库**不绑定**某一产品名或学科。实现时按上文步骤新建 `internal/service/{yourmodule}`，不要修改脚手架已有的 auth/user 核心表语义，除非你有意升级全平台用户模型。
