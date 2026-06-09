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

管理端统一挂在 `router.go` 的 `admin := v1.Group("/admin", middleware.Auth(), middleware.RequireRole("admin"))`，真实业务只需要继续注册到该分组。

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
- 日志：`log.level`、`log.format`
- 限流：`rate_limit.per_minute`
- CORS：`cors.allow_origins`、`cors.allow_credentials`
- Auth 固定验证码：`auth.mock_code_enabled`
- OAuth：`oauth.dev_mode`、`oauth.providers`

启动时会执行安全校验：生产环境禁止默认 `JWT_SECRET`，禁止启用固定验证码/OAuth 开发模式，且必须显式配置 CORS 来源。

## 登出与会话语义

`POST /api/v1/auth/logout` 需提交 `refresh_token`，服务端会：

1. 将 refresh token 写入 Redis 黑名单（后续 Refresh 拒绝）。
2. 按 `user_id` 写入 access token 吊销时间戳（`Auth` 中间件比对 `iat`）。

**行为约定：**

- **全端登出**：任意设备登出会使该账号在所有设备上、登出时刻之前签发的 access token 失效。适合教育类等「单账号」场景；多设备独立会话需改为 per-token 吊销。
- **失败即报错**：Redis 写入失败时 service 返回 `ErrInternal`，HTTP 层不再静默返回 200。
- **吊销检查 fail-open**：Redis 宕机时 access token 吊销检查放行并打 Warn 日志，避免 Redis 故障拖垮全站鉴权；极高安全需求可自行改为 fail-closed。

## OAuth（骨架）

开发模式（`oauth.dev_mode=true`）下，`POST /api/v1/auth/login` 支持：

```json
{
  "grant_type": "oauth",
  "provider": "wechat",
  "open_id": "wx_openid_xxx"
}
```

流程：按 `provider + open_id` 查 `user_oauth` → 已绑定则登录 → 未绑定则自动创建用户并写入绑定记录。

生产环境需关闭 `oauth.dev_mode`，并在 `service/auth` 的 OAuth 分支接入真实 token 校验逻辑。

## 中间件顺序

`RequestID` → `I18n` → `Zap` → `Recovery` → `Safe` → `RateLimit` → `CORS` → 路由级 `Auth` / `RequireRole`。

## 健康检查

- `/health`：轻量存活检查，只说明进程可响应。
- `/ready`：就绪检查，会 ping PostgreSQL 与 Redis，适合容器编排 readiness。

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

CI 位于 `.github/workflows/ci.yml`，会校验 sqlc 生成代码、运行测试并构建后端。

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

本仓库**不绑定**某一产品名或学科。历史业务 API 设计放在 `docs/examples/`，实现时按上文步骤新建 `internal/service/{yourmodule}`，不要修改脚手架已有的 auth/user 核心表语义，除非你有意升级全平台用户模型。
