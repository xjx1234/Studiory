# 部署与启动

本项目提供两种启动方式：**Docker 一键全栈**（推荐用于快速体验/演示）和**本地开发**（推荐用于日常编码）。

## 一、Docker 一键全栈

一条命令启动 API + PostgreSQL + Redis，并在 API 启动前自动完成数据库迁移：

```bash
make up
```

等价于：

```bash
docker compose --profile full up -d --build
```

启动流程（由 `depends_on` 编排）：

1. `postgres`、`redis` 启动并通过健康检查
2. `migrate` 一次性任务对 PostgreSQL 执行 `migrations/` 下的迁移，成功后退出
3. `api` 构建镜像并启动，监听 `:8080`

验证：

```bash
curl http://localhost:8080/health
# {"code":0,"message":"...","data":{"status":"ok","service":"api","version":"...","commit":"...","build_time":"..."}}

curl http://localhost:8080/ready
# 探测 PostgreSQL + Redis 连通性
```

查看 API 日志、停止：

```bash
make logs
make down
```

### 初始化管理员

全栈启动后，可在容器内或本地创建 admin 账号：

```bash
SEED_ADMIN_PHONE=13800000000 SEED_ADMIN_PASSWORD=YourStrongPass make seed
```

> `make seed` 默认连接 `localhost:5432`。compose 已将 PostgreSQL 端口映射到宿主机，可直接运行。

### 说明

- compose 中的 `api` 默认以 `APP_ENV=development` 运行，启用 mock 验证码（固定 `123456`）、OAuth dev 模式与默认 CORS，**开箱即用**。
- 默认 **多设备登录**（`AUTH_MULTI_DEVICE_ENABLED=true`），同一账号可在多台设备同时保持登录。
- `api` 与 `migrate` 服务使用 compose `profiles: ["full"]`，因此 `make compose-up` 只会启动 PostgreSQL + Redis，不会构建 API。
- 数据库迁移**不打入** API 镜像，由独立的 `migrate` 任务执行。生产环境应改由 CI/CD 流水线或 Kubernetes Job/initContainer 执行迁移。

## 二、本地开发

只用 Docker 起依赖，API 在本地用 Go 运行，便于热改与调试：

```bash
# 1. 启动 PostgreSQL + Redis
make compose-up

# 2. 执行数据库迁移（需本地安装 golang-migrate CLI）
make migrate-up

# 3. 准备配置
cp backend/.env.example backend/.env   # 按需修改

# 4. 运行 API
make run
```

## 三、生产部署要点

生产环境以 `APP_ENV=production` 运行，启动时会做配置校验（`config.Validate()`），必须满足：

| 配置 | 要求 |
|------|------|
| `JWT_SECRET` | 不能为默认值 `dev-secret-change-in-production` |
| `AUTH_MOCK_CODE_ENABLED` | 必须为 `false` |
| `OAUTH_DEV_MODE` | 必须为 `false` |
| `CORS_ALLOW_ORIGINS` | 至少配置一个来源 |
| `RATE_LIMIT_PER_MINUTE` | 必须大于 0（公开路由按 IP） |
| `RATE_LIMIT_USER_PER_MINUTE` | 可选；未配置时默认与 `RATE_LIMIT_PER_MINUTE` 相同（已鉴权路由按 user_id） |
| `DATABASE_URL` / `REDIS_URL` | 不能为空 |
| `AUTH_MULTI_DEVICE_ENABLED` | 按需设置（见下方「多/单设备 Session」） |
| HTTP 超时 | `read_timeout` ≥ `read_header_timeout`，四项均须 > 0（见下方） |

### HTTP Server 超时

`http.Server` 的四项超时由 `app.*_timeout` 配置（环境变量 `SERVER_*_TIMEOUT`），默认值见 `config/base.yaml`：

| 配置项 | 环境变量 | 默认 | 作用 |
|--------|----------|------|------|
| `app.read_header_timeout` | `SERVER_READ_HEADER_TIMEOUT` | `5s` | 防 Slowloris，限制读取请求头 |
| `app.read_timeout` | `SERVER_READ_TIMEOUT` | `15s` | 限制读取完整请求（含 body） |
| `app.write_timeout` | `SERVER_WRITE_TIMEOUT` | `30s` | 限制写入响应 |
| `app.idle_timeout` | `SERVER_IDLE_TIMEOUT` | `120s` | keep-alive 空闲连接存活时间 |

生产环境一般保持默认即可；若后续接入大文件上传，可适当调大 `read_timeout` / `write_timeout`。

### 多/单设备 Session

通过 `AUTH_MULTI_DEVICE_ENABLED`（对应 `auth.multi_device_enabled`）控制同一账号是否允许多设备同时在线。详见 [session.md](session.md)。

| 值 | 行为 | 适用场景 |
|----|------|----------|
| `true`（默认） | 多设备：手机、电脑可同时登录，登出仅影响当前设备 | 通用 C 端产品、办公协作 |
| `false` | 单设备：新登录立即踢掉旧会话，旧设备 token 失效 | 高安全账号、单端独占 |

配置方式（任选其一）：

```yaml
# config/production.yaml 或 config/local.yaml
auth:
  multi_device_enabled: false
```

```bash
# 环境变量
AUTH_MULTI_DEVICE_ENABLED=false
```

本地验证单设备模式：

```bash
# 1. 设 AUTH_MULTI_DEVICE_ENABLED=false 后启动 API
# 2. 同一账号登录两次，分别拿到 token-A、token-B
# 3. 用 token-A 请求 /api/v1/user/profile → 401
# 4. 用 token-B 请求 /api/v1/user/profile → 200
```

> 改密、后台禁用账号会吊销**全部** session（与单/多设备模式无关）。Redis 键说明见 [redis-keys.md](redis-keys.md) 第 7 节。

构建生产镜像（注入版本信息）：

```bash
docker build \
  --build-arg VERSION=v1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  -t studiory-api:v1.0.0 \
  -f backend/Dockerfile backend
```

迁移独立执行（示例，使用与 compose 相同的 migrate 镜像）：

```bash
docker run --rm \
  -v "$(pwd)/backend/migrations:/migrations:ro" \
  migrate/migrate:v4.17.0 \
  -path /migrations \
  -database "$DATABASE_URL" up
```
