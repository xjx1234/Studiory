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
| `AUTH_REQUIRE_CODE_FOR_PASSWORD_REGISTER` | 必须为 `true`（默认值）：密码注册要先通过一次性验证码，证明持有该手机号/邮箱 |
| `OAUTH_DEV_MODE` | 必须为 `false` |
| `OAUTH_GOOGLE_CLIENT_ID` | 启用 google 登录时必填（强制校验 `aud`） |
| `OAUTH_APPLE_CLIENT_ID` | 启用 apple 登录时必填 |
| `OAUTH_WECHAT_APP_ID` | 启用 wechat 登录时必填 |
| `METRICS_TOKEN` | `METRICS_ENABLED=true` 时必填（`/metrics` bearer token） |
| `AUTH_REDIS_FAIL_OPEN` | 生产默认 `false`（安全优先）；可用性优先可设为 `true` |
| `CORS_ALLOW_ORIGINS` | 至少配置一个来源；`CORS_ALLOW_CREDENTIALS=true` 时不能包含 `*` |
| `REDIS_KEY_PREFIX` | 不能为空，不能含冒号或空白 |
| `REDIS_READ_TIMEOUT` / `REDIS_WRITE_TIMEOUT` | 必须大于 0（不再依赖 go-redis 默认值） |
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
| `app.request_timeout` | `SERVER_REQUEST_TIMEOUT` | `5s` | 单请求 context 超时（DB/Redis 共享），须小于 `write_timeout`；超时统一 503 |

生产环境一般保持默认即可；若后续接入大文件上传，可适当调大 `read_timeout` / `write_timeout`。

连接池与依赖超时：

| 配置项 | 环境变量 | 默认 | 作用 |
|--------|----------|------|------|
| `database.statement_timeout` | `DB_STATEMENT_TIMEOUT` | `3s` | PostgreSQL 语句超时，防慢查询占满连接池（MySQL 驱动下暂不生效） |
| `redis.read_timeout` | `REDIS_READ_TIMEOUT` | `3s` | 单次 Redis 读超时 |
| `redis.write_timeout` | `REDIS_WRITE_TIMEOUT` | `3s` | 单次 Redis 写超时 |

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

### 阻塞性迁移的前置检查

**检查什么**：会被唯一索引或迁移预检拦下的历史数据只有两类——归一化后相同的邮箱、归一化后相同的手机号。
归一化口径与服务层一致：邮箱是 `lower(btrim(email))`；手机号先去非数字字符，再依次剥 `0086` / `86` 前缀
（仅当剩余部分匹配 `1[3-9]\d{9}`，避免误改其他国家号码）。

**发布前怎么跑**：**存在存量库时必须先跑**（每次要执行 000005 / 000007 的环境都要跑，不是一次性动作）。
两个脚本都是只读查询、口径与迁移预检完全一致，不要在别处复制 SQL：

```bash
psql "$DATABASE_URL" -f scripts/check_phone_collisions.sql
psql "$DATABASE_URL" -f scripts/check_email_collisions.sql
```

两条命令都没有输出，才可以发布。各执行路径的接入位置：
K8s 见[部署步骤第 3.5 步](#部署步骤示例)；compose 一键栈见 `docker-compose.yml` 的 `migrate` 服务注释；
本地开发在 `make migrate-up` 前手动跑一次（命令会提示）。

**撞了怎么处置**：人工决定每组保留哪一行——登录频率高、绑定 OAuth、业务数据多的账号优先保留。
其余账号要么合并（把 `user_oauth`、业务表里的 `user_id` 改指保留行后再删），要么直接停用
（`status = 'disabled'`）而不是物理删除，避免外键与审计信息丢失。处理完重新跑上面的检查，直到无输出。

### 迁移失败处置（dirty 状态）

golang-migrate 不记录迁移文件的 checksum，只在 `schema_migrations` 里记录 `version` 与 `dirty`；
执行中途失败会停在「目标版本 + `dirty=true`」。先确认当前版本：

```bash
migrate -path backend/migrations -database "$DATABASE_URL" version
# 例：7 (dirty)
```

然后 `force` 到该版本的**前一个成功版本**再重跑——不要照抄示例里的数字，以命令输出为准：

```bash
migrate -path backend/migrations -database "$DATABASE_URL" force 6
migrate -path backend/migrations -database "$DATABASE_URL" up
```

> 锚点：当前阻塞性失败点有两个——`000005`（重复邮箱导致 `users_email_lower_key` 建不出来）与
> `000007`（手机号清洗预检撞号）。上面的示例按 000007 失败写成 `force 6`；新增迁移版本后
> 请以 `migrate version` 的输出为准，不要直接沿用文档里的数字。

不要把 `force` 到失败版本本身当成解决办法：
- 跳过 `000005`：`users_email_lower_key` 实际没有建成，大小写重复注册的口子会继续存在；
- 跳过 `000007`：数据库停在 version 6，手机号仍是带分隔符/国家码的写法，服务层按裸 11 位查询不到这些账号。

确认要承担后果时才跳过，并在后续版本里补偿。

### 迁移不可变闸门（GitHub 侧）

CI 的 `Migrations` workflow 会以**最早可达**的 `migrations-baseline-*` tag 为基线，对比 HEAD 检查
`backend/migrations/**` 是否只新增文件（脚本：[scripts/check_migrations_immutable.sh](../scripts/check_migrations_immutable.sh)）。
无 tag → 失败；有 tag 但都不在当前历史里（分支未 rebase）→ 失败并提示 rebase；首次接入用
`MIGRATIONS_BASELINE_BOOTSTRAP=1` 显式放行一次，打完基线 tag 后必须删除该变量。

仓库侧还必须加一道设置，否则有写权限的人可以在自己提交上自打一个基线 tag 绕过检查：

> **GitHub → Settings → Rules → Rulesets → New tag ruleset**
> - Target tags：`migrations-baseline-*`
> - 勾选 **Restrict creations** 与 **Restrict deletions**，Bypass list 只保留管理员/组织 owner。

注意：`git push origin <tag>` 默认不受分支保护限制，没有这条规则时任何有写权限的账号都能推送 tag。
设置后，正常发版打基线需要管理员操作，绕过闸门也就无法做到“无痕”。

## 四、Kubernetes 部署示例

示例清单位于 [`deploy/k8s/`](../deploy/k8s/)，包含 Deployment（探针 + 资源限制 + 优雅下线 + 反亲和）、Service、ConfigMap（非敏感配置）、Ingress（域名 + TLS）、PodDisruptionBudget、ServiceMonitor（可选）、迁移 Job 与 Secret 模板。

### 探针约定

| 探针 | 路径 | 作用 | 失败后果 |
|------|------|------|----------|
| **liveness** | `GET /health` | 进程是否存活（不查 PG/Redis） | kubelet **重启** Pod |
| **readiness** | `GET /ready` | 并发探测 PG + Redis | 从 Service **摘除**流量，不重启 |

`/ready` 并发执行所有探针（共享 2s 预算），响应 `data.checks` 逐项返回 `name`、`ok`、`latency_ms`；
任一失败即 503，但不会中断其他探针——运维能直接看出是哪个依赖挂了。失败原因只回显「不可用/未初始化」，
底层错误（连接串、内网地址）只写日志，避免从无鉴权的探活端点泄露内部信息。

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
  但 `METRICS_ENABLED=true` 时 `/metrics` 需要 Bearer token，annotation 本身不携带凭据，scrape_config 必须补上鉴权：

  ```yaml
  scrape_configs:
    - job_name: studiory-api
      kubernetes_sd_configs: [{ role: pod }]
      authorization:
        type: Bearer
        credentials_file: /etc/prometheus/secrets/metrics-token   # 内容即 METRICS_TOKEN
      relabel_configs:
        - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
          action: keep
          regex: "true"
  ```

  也可以改用 `bearer_token_file`；两种写法都需要把 Secret 里的 `METRICS_TOKEN` 挂进 Prometheus。

  > ⚠️ `secret.example.yaml` 里的 `METRICS_TOKEN` 是公开的占位值，必须在部署前替换。
  > 示例 [`ingress.yaml`](../deploy/k8s/ingress.yaml) 以 `path: /` 转发全部路径，`/metrics` 也会被暴露到公网；
  > 未替换 token 之前，建议在 Ingress 层单独 deny `/metrics`，或把 Ingress 收敛为只转发 `/api`。

- **Prometheus Operator**：应用 [`service-monitor.yaml`](../deploy/k8s/service-monitor.yaml)（需要集群已安装
  `ServiceMonitor` CRD）。该文件已声明 `bearerTokenSecret` 指向 `studiory-api-secret` 的 `METRICS_TOKEN`，
  与 Secret 模板、启动校验三处使用同一个键名：

```bash
kubectl apply -f deploy/k8s/service-monitor.yaml
```

### ConfigMap 与 Secret（配置分离）

配置分为两类，通过 `envFrom` 组合注入容器：

| 来源 | 文件 | 存放内容 | 更新方式 |
|------|------|----------|----------|
| **ConfigMap** | [`configmap.yaml`](../deploy/k8s/configmap.yaml) | 非敏感配置：`APP_ENV`、`LOG_FORMAT`、`CORS_ALLOW_ORIGINS`、`RATE_LIMIT_*`、`SERVER_TRUSTED_PROXIES` 等 | `kubectl apply -f` |
| **Secret** | [`secret.example.yaml`](../deploy/k8s/secret.example.yaml) | 敏感信息：`DATABASE_URL`、`REDIS_URL`、`JWT_SECRET`、`METRICS_TOKEN`、`OAUTH_*` 等 | 复制为 `secret.yaml` 编辑后 apply |

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

#### 让应用认识真实客户端 IP（`SERVER_TRUSTED_PROXIES`）

Gin 默认信任所有代理，本项目已改为按 `SERVER_TRUSTED_PROXIES` 收紧：不配置时**不信任任何代理**，
`X-Forwarded-For` 被完全忽略，IP 限流与访问日志只认 TCP 对端地址。这在直连场景是安全的，
但经 Ingress 转发时所有请求的对端都是 Ingress Pod，用户会共用同一个限流桶（启动日志会给出 Warn）。

示例 ConfigMap 因此**预置了一组可用默认值**，而不是留空：

```yaml
# deploy/k8s/configmap.yaml
data:
  SERVER_TRUSTED_PROXIES: "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
```

选择理由：

- **为什么给默认值而不是设为必填**：设为必填会让"照抄清单部署"重新变成启动失败或全站单桶，
  与本模板"开箱可跑"的目标冲突；而写死某个 CNI 网段（如 `10.244.0.0/16`）只对 Flannel 成立，
  Calico 默认的 `192.168.0.0/16`、k3s 的 `10.42.0.0/16` 都会落空。
- **为什么是 RFC1918 而不是 `0.0.0.0/0`**：公网来源永远不在列表里，外部客户端无法通过伪造
  `X-Forwarded-For` 绕过限流——这正是原始漏洞的利用面；私网段覆盖了常见 CNI 的 Pod/Node 地址。
- **代价**：如果 TCP 对端落在私网（例如办公网直连 NodePort），该客户端仍可伪造 XFF。
  生产应把列表收窄到真实代理网段来消除这一点：

```yaml
# 例：只看 ingress-nginx 的 Pod CIDR
SERVER_TRUSTED_PROXIES: "10.244.0.0/16"
```

填写原则：只写真正可控的代理地址（CNI Pod/Node CIDR 或上游 LB 的 IP 段）。
切勿写 `0.0.0.0/0` —— 那等于把 Gin 的出厂隐患放回来，任何人都能用一个请求头指定自己的限流桶。
ingress-nginx 默认会把真实客户端追加进 `X-Forwarded-For`，Gin 从右向左跳过可信代理后取到的
就是真实客户端，因此 Ingress 侧无需额外 annotations；改完记得
`kubectl rollout restart deployment/studiory-api`。

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
# 必须替换其中的占位值：JWT_SECRET、METRICS_TOKEN（可用 `openssl rand -hex 32` 生成）、
# DATABASE_URL、REDIS_URL，以及 ConfigMap 的 OAUTH_PROVIDERS 中每个渠道对应的凭据。
# 不需要某个 OAuth 渠道时，从 configmap.yaml 的 OAUTH_PROVIDERS 里移除它即可。
# 编辑 secret.yaml 后：
kubectl apply -f secret.yaml

# 3.5 存量库前置检查（必须先跑，否则第 4 步迁移会 dirty）
# 连接方式固定为 port-forward：把 PG Service 临时映射到本机 5433，在仓库根目录用只读脚本检查。
# Service 名/命名空间按实际环境替换（示例假设 PG 在 default 命名空间、Service 名为 postgres）。
kubectl port-forward svc/postgres 5433:5432 >/dev/null 2>&1 &
PF_PID=$!
psql "postgres://USER:PASSWORD@localhost:5433/app?sslmode=disable" -f scripts/check_phone_collisions.sql
psql "postgres://USER:PASSWORD@localhost:5433/app?sslmode=disable" -f scripts/check_email_collisions.sql
kill $PF_PID
# 两个脚本都必须无输出才能继续；有输出先按「阻塞性迁移的前置检查」处置，不能直接跑第 4 步。

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
