# Go API 脚手架

一套可复用的 Go 后端 API 脚手架：统一 HTTP API 入口，技术栈固定，**不包含具体业务**，新项目克隆后按模块扩展即可。

## 顶层结构

```text
.
├── backend/          # Golang 后端（统一 API，独立部署）
│   ├── config/       # 分层 YAML 配置
│   ├── migrations/   # PostgreSQL 迁移
│   ├── internal/     # 业务实现（分层见 docs/architecture.md）
│   ├── pkg/          # 可复用公共库
│   └── main.go
├── docs/             # 架构说明、Redis 键规范、部署、示例 API 等
├── deploy/k8s/       # Kubernetes 示例清单（探针、资源、迁移 Job）
├── docker-compose.yml
└── Makefile
```

## 后端分层（概要）

| 层 | 目录 | 职责 |
|----|------|------|
| 入口 | `main.go` | 日志、配置、信号、启停 |
| 装配 | `internal/app` | 连接 PG/Redis，注入 Service，挂路由 |
| HTTP | `internal/http` | 路由、中间件、Handler（薄层） |
| 业务 | `internal/service/*` | 用例逻辑，编排 repo |
| 数据 | `internal/repo` | 接口 + `pg` 实现 + `sqlc` 生成 |
| 基础设施 | `internal/auth`、`internal/store`、`pkg/*` | JWT、连接池、统一响应等 |

**开箱即用能力**：健康检查、就绪检查、注册/登录/验证码/刷新/登出、用户资料、角色权限中间件。

**示例模块**：
- Todo CRUD（`/api/v1/user/todos`），演示完整分层，见 [docs/examples/todo-module.md](docs/examples/todo-module.md)。
- Admin 用户管理（`/api/v1/admin/users`），演示 RBAC 后台管理与禁用即时吊销，见 [docs/examples/admin-user-management.md](docs/examples/admin-user-management.md)。

详细约定与「如何加一个新模块」见 [docs/architecture.md](docs/architecture.md)。

## 快速开始

### 方式一：Docker 一键全栈（推荐快速体验）

构建并启动 API + PostgreSQL + Redis，启动前自动执行数据库迁移：

```bash
make up                              # 全栈启动（含自动迁移）
curl http://localhost:8080/health    # 验证
make logs                            # 查看 API 日志
make down                            # 停止
```

### 方式二：本地开发（推荐日常编码）

仅用 Docker 起依赖，API 在本地用 Go 运行：

```bash
# 1. 配置（config.Load 会自动加载 backend/.env）
cp backend/.env.example backend/.env
# 编辑 DATABASE_URL、REDIS_URL、JWT_SECRET

# 2. 启动本地依赖（仅 PostgreSQL + Redis）
make compose-up

# 3. 数据库迁移（需本地安装 golang-migrate CLI）
make migrate-up

# 4. 启动后端
make run
```

完整启动/部署说明（含生产配置校验、独立迁移）见 [docs/deploy.md](docs/deploy.md)。

开发环境默认启用固定验证码 `123456`（`AUTH_MOCK_CODE_ENABLED=true`）。生产环境会禁止启用该开关，并要求替换默认 `JWT_SECRET`。

常用命令：

```bash
make test        # go test ./...
make build       # 构建后端
make sqlc        # 生成 sqlc 代码
make migrate-up  # 执行迁移
```

## 数据库与 Redis

- **PostgreSQL**：`users`、`user_oauth`，以及示例表 `todos`。新业务表通过新 migration 添加。
- **sqlc**：`backend/sqlc.yaml`，查询写在 `internal/repo/sqlc/query/`，生成到 `gen/`。说明见 `backend/docs/sqlc.md`。
- **Redis**：验证码、Refresh Token 黑名单等，键前缀由 `REDIS_KEY_PREFIX` 控制，见 [docs/redis-keys.md](docs/redis-keys.md)。

## 健康检查与权限

- `GET /health`：进程存活检查，不访问外部依赖。
- `GET /ready`：检查 PostgreSQL 与 Redis，可用于容器 readiness。
- `GET /api/v1/admin/ping`：admin 权限示例，需 JWT 且 `role=admin`。

## 常用业务 API

- `GET /api/v1/user/profile`：获取当前登录用户资料
- `PATCH /api/v1/user/profile`：更新当前登录用户资料
- `GET /api/v1/user/todos?page=&page_size=`：待办列表（分页）
- `POST /api/v1/auth/login`（`grant_type=oauth`）：第三方登录，见 [docs/oauth.md](docs/oauth.md)

## 示例文档

`docs/examples/` 存放脚手架自带示例模块的说明（如 Todo CRUD）。

- 示例模块：[docs/examples/todo-module.md](docs/examples/todo-module.md)、[docs/examples/admin-user-management.md](docs/examples/admin-user-management.md)
- OAuth 登录：[docs/oauth.md](docs/oauth.md)
- 多/单设备 Session：[docs/session.md](docs/session.md)
- 验证码下发：[docs/code-sender.md](docs/code-sender.md)
- 结构化日志：[docs/logging.md](docs/logging.md)
- 指标：[docs/metrics.md](docs/metrics.md)
- OpenAPI：`docs/api/openapi.yaml`
- 错误码：`docs/api/errcode.md`

## 新项目 checklist

1. 改 `backend/go.mod` 的 module 名与 import 路径。
2. 改 `config/base.yaml`、`backend/.env` 中的库名、JWT 密钥。
3. 改 `REDIS_KEY_PREFIX`、`CORS_ALLOW_ORIGINS`，避免多项目互相影响。
4. 按需调整 `users.role` 枚举（migration）。
5. 按 [docs/architecture.md](docs/architecture.md) 增加 migration → sqlc → repo → service → http。
6. 评估示例 Todo 模块：保留作参考，或按 [docs/examples/todo-module.md](docs/examples/todo-module.md) 删除。
