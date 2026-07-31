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
| `JWT_SECRET` | 不能为默认值 `dev-secret-change-in-production`，长度 ≥32 字节 |
| `AUTH_MOCK_CODE_ENABLED` | 必须为 `false` |
| `OAUTH_DEV_MODE` | 必须为 `false` |
| `OAUTH_GOOGLE_CLIENT_ID` | 启用 google 登录时必填（强制校验 `aud`） |
| `OAUTH_APPLE_CLIENT_ID` | 启用 apple 登录时必填 |
| `OAUTH_WECHAT_APP_ID` | 启用 wechat 登录时必填 |
| `METRICS_TOKEN` | `METRICS_ENABLED=true` 时必填（`/metrics` bearer token） |
| `AUTH_REDIS_FAIL_OPEN` | 生产默认 `false`（安全优先）；可用性优先可设为 `true` |
| `CORS_ALLOW_ORIGINS` | 至少配置一个来源 |
| `RATE_LIMIT_PER_MINUTE` | 必须大于 0（公开路由按 IP） |
| `RATE_LIMIT_USER_PER_MINUTE` | 可选；未配置时默认与 `RATE_LIMIT_PER_MINUTE` 相同（已鉴权路由按 user_id） |
| `DATABASE_URL` / `REDIS_URL` | 不能为空；`DATABASE_URL` 必须显式携带 `sslmode=require/verify-ca/verify-full`（防止 TLS 被静默绕过） |
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

## 四、Kubernetes 部署示例

示例清单位于 [`deploy/k8s/`](../deploy/k8s/)，包含 Deployment（探针 + 资源限制 + 优雅下线 + 反亲和）、Service、ConfigMap（非敏感配置）、Ingress（域名 + TLS）、PodDisruptionBudget、ServiceMonitor（可选）、迁移 Job 与 Secret 模板。

### 探针约定

| 探针 | 路径 | 作用 | 失败后果 |
|------|------|------|----------|
| **liveness** | `GET /health` | 进程是否存活（不查 PG/Redis） | kubelet **重启** Pod |
| **readiness** | `GET /ready` | PG + Redis 是否可用 | 从 Service **摘除**流量，不重启 |

与 Docker 镜像内置 `HEALTHCHECK`（仅 `/health`）一致；K8s 额外用 `/ready` 做依赖就绪判断，避免数据库未连通时仍接收请求。

推荐参数（已写入 `api-deployment.yaml`，可按集群调整）：

| 参数 | liveness | readiness | 说明 |
|------|----------|-----------|------|
| `initialDelaySeconds` | 10 | 5 | 启动宽限期 |
| `periodSeconds` | 30 | 10 | 检查间隔 |
| `timeoutSeconds` | 3 | 3 | 单次超时（与 `SERVER_READ_HEADER_TIMEOUT=5s` 兼容） |
| `failureThreshold` | 3 | 3 | 连续失败次数 |

> 若冷启动较慢（大镜像、节点资源紧张），可增大 `initialDelaySeconds` 或增加 `startupProbe`（先探测 `/health`，成功后再启用 liveness/readiness）。

### 资源限制

`api-deployment.yaml` 默认：

| | CPU | 内存 |
|---|-----|------|
| requests | 100m | 128Mi |
| limits | 500m | 512Mi |

Go API 通常内存占用不高；高并发或大量连接时可按 Prometheus / 压测结果调高 `limits`，并同步调整 `database.pool.max_conns`、`redis.pool_size`。

### 优雅下线（preStop + terminationGracePeriodSeconds）

滚动发布/缩容时，kubelet 发 SIGTERM 与 Service/kube-proxy/Ingress 摘除该 Pod 的 Endpoint 是**并行**发生的，
没有严格的先后顺序保证——如果 App 收到 SIGTERM 立刻退出，仍可能有新请求在摘流生效前被转发进来。

`api-deployment.yaml` 用一个简单的 `preStop: sleep 5` 缓解：

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sh", "-c", "sleep 5"]
```

时间线：`preStop`（5s，只 sleep 不处理业务）→ kubelet 发 SIGTERM → 应用内优雅关闭（`main.go` 里 shutdown ctx 10s）。
`terminationGracePeriodSeconds` 必须覆盖这两段时间之和，否则会在关闭完成前被 kubelet 强杀（SIGKILL），
所以清单里设为 `20`（5 + 10 + 余量）。若调整了 shutdown 超时或 preStop 时长，记得同步调整这个值。

### 反亲和（podAntiAffinity）与 PodDisruptionBudget

- **podAntiAffinity**（软约束）：尽量把 `studiory-api` 的多个副本调度到不同节点，避免单节点故障时全部实例同时下线。用
  `preferredDuringSchedulingIgnoredDuringExecution` 而非 `required`，小集群节点数不够时也能正常调度，只是不保证严格分散。
- **PodDisruptionBudget**（[`pdb.yaml`](../deploy/k8s/pdb.yaml)）：限制节点维护 drain、`cluster-autoscaler` 缩容等
  **自愿驱逐**同时踢掉的副本数（`minAvailable: 1`），不影响 kubelet 对故障 Pod 的强制驱逐，也不影响 Deployment
  自身滚动发布。副本数调多后建议改成百分比形式的 `maxUnavailable`。

```bash
kubectl apply -f deploy/k8s/pdb.yaml
```

### Prometheus 抓取

`/metrics` 的抓取方式二选一，不要同时启用：

- **经典 Prometheus**（`kubernetes_sd_configs`）：`api-deployment.yaml` 的 Pod template 已经带了
  `prometheus.io/scrape`、`prometheus.io/port`、`prometheus.io/path` annotation，配合对应的 relabel 规则即可自动发现。
- **Prometheus Operator**：应用 [`service-monitor.yaml`](../deploy/k8s/service-monitor.yaml)（需要集群已安装
  `ServiceMonitor` CRD）：

```bash
kubectl apply -f deploy/k8s/service-monitor.yaml
```

### ConfigMap 与 Secret（配置分离）

配置分为两类，通过 `envFrom` 组合注入容器：

| 来源 | 文件 | 存放内容 | 更新方式 |
|------|------|----------|----------|
| **ConfigMap** | [`configmap.yaml`](../deploy/k8s/configmap.yaml) | 非敏感配置：`APP_ENV`、`LOG_FORMAT`、`CORS_ALLOW_ORIGINS`、`RATE_LIMIT_*` 等 | `kubectl apply -f` |
| **Secret** | [`secret.example.yaml`](../deploy/k8s/secret.example.yaml) | 敏感信息：`DATABASE_URL`、`REDIS_URL`、`JWT_SECRET` 等 | 复制为 `secret.yaml` 编辑后 apply |

```bash
kubectl apply -f deploy/k8s/configmap.yaml

cp deploy/k8s/secret.example.yaml secret.yaml
# 编辑 secret.yaml 后：
kubectl apply -f secret.yaml
```

> ConfigMap 更新后不会自动重启 Pod，需手动触发：`kubectl rollout restart deployment/studiory-api`

### Ingress（域名 + TLS）

[`ingress.yaml`](../deploy/k8s/ingress.yaml) 提供了基于 ingress-nginx 的 Ingress 模板，包含：

- HTTPS 重定向（`ssl-redirect: true`）
- 请求体大小限制（`proxy-body-size: 10m`）
- 限速（`limit-rps: 10`，`limit-burst: 20`）
- 超时与后端 `SERVER_READ/WRITE_TIMEOUT` 对齐
- TLS 证书引用（手动创建或 cert-manager 自动签发）

使用前需修改 `spec.rules.host` 和 `spec.tls.hosts` 为实际域名，并确保集群已安装 Ingress Controller。

```bash
# 安装 ingress-nginx（如尚未安装）
helm install ingress-nginx ingress-nginx/ingress-nginx \
  -n ingress-nginx --create-namespace

# TLS 证书（二选一）
# 方式一：手动创建
kubectl create secret tls studiory-tls \
  --cert=path/to/tls.crt --key=path/to/tls.key

# 方式二：cert-manager 自动签发（需先安装 cert-manager）
# 参考 https://cert-manager.io/docs/installation/

# 应用 Ingress
kubectl apply -f deploy/k8s/ingress.yaml
```

> 如果使用云厂商 Ingress Controller（如 AWS ALB、GCP GCE），需调整 `ingressClassName` 和 annotations。

### 部署步骤（示例）

```bash
# 1. 构建并推送镜像
docker build \
  --build-arg VERSION=v1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  -t registry.example.com/studiory-api:v1.0.0 \
  -f backend/Dockerfile backend
docker push registry.example.com/studiory-api:v1.0.0

# 2. 创建 ConfigMap（非敏感配置）
kubectl apply -f deploy/k8s/configmap.yaml

# 3. 创建 Secret（勿提交真实 secret.yaml）
cp deploy/k8s/secret.example.yaml secret.yaml
# 编辑 secret.yaml 后：
kubectl apply -f secret.yaml

# 4. 迁移（先于 API 上线）
kubectl create configmap studiory-migrations \
  --from-file=backend/migrations/ \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f deploy/k8s/migrate-job.yaml
kubectl wait --for=condition=complete job/studiory-migrate --timeout=120s

# 5. 部署 API
# 编辑 deploy/k8s/api-deployment.yaml 中的 image 后：
kubectl apply -f deploy/k8s/api-service.yaml
kubectl apply -f deploy/k8s/api-deployment.yaml
kubectl apply -f deploy/k8s/pdb.yaml
# 若使用 Prometheus Operator：
# kubectl apply -f deploy/k8s/service-monitor.yaml

# 6. 配置 Ingress（域名 + TLS）
# 编辑 deploy/k8s/ingress.yaml 中的 host 和 TLS 配置后：
kubectl apply -f deploy/k8s/ingress.yaml

# 7. 验证
kubectl rollout status deployment/studiory-api
kubectl port-forward svc/studiory-api 8080:80 &
curl -s http://localhost:8080/health
curl -s http://localhost:8080/ready
```

### 说明

- PostgreSQL / Redis 需由集群内其他 Helm Chart 或托管服务提供；Secret 中的 `DATABASE_URL`、`REDIS_URL` 指向对应地址。
- 生产务必设置 `APP_ENV=production` 及第三节所列安全项；`secret.example.yaml` 已默认关闭 mock 验证码与 OAuth dev 模式。
- `/metrics` 生产环境需配置 `METRICS_TOKEN` bearer token（应用层保护），并通过 NetworkPolicy / 内网 ServiceMonitor 限制访问，见 [metrics.md](metrics.md)。
- 滚动发布时，旧 Pod 在 `readinessProbe` 失败期间自动摘流；新 Pod 通过 `/ready` 后才接流量。
