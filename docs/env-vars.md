# 环境变量参考

所有环境变量均可通过 `backend/.env` 文件或系统环境变量注入。配置加载优先级：

```
环境变量 > local.yaml > {env}.yaml > base.yaml
```

> 完整配置模板见 [`backend/.env.example`](../backend/.env.example)。

---

## 服务

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `APP_ENV` | `development` | 否 | 运行环境：`development` / `production` / `test` |
| `SERVER_ADDR` | `:8080` | 否 | HTTP 监听地址 |
| `SERVER_READ_HEADER_TIMEOUT` | `5s` | 否 | 读取请求头超时（防 Slowloris） |
| `SERVER_READ_TIMEOUT` | `15s` | 否 | 读取完整请求（含 body） |
| `SERVER_WRITE_TIMEOUT` | `30s` | 否 | 写入响应超时 |
| `SERVER_IDLE_TIMEOUT` | `120s` | 否 | keep-alive 空闲连接超时 |
| `SERVER_REQUEST_TIMEOUT` | `5s` | 否 | 单请求 context 超时（DB/Redis 调用共享该 ctx）。必须小于 `SERVER_WRITE_TIMEOUT`，超时后统一返回 503 |
| `SERVER_SEND_CODE_REQUEST_TIMEOUT` | `20s` | 否 | 仅 `/api/v1/auth/send-code` 的请求预算（同步外呼短信/邮件网关）。`0` 或留空 = 沿用 `SERVER_REQUEST_TIMEOUT`。约束：`SERVER_REQUEST_TIMEOUT` ≤ 该值 ≤ `SERVER_WRITE_TIMEOUT`，且 > `SMTP_DIAL_TIMEOUT + SMTP_SEND_TIMEOUT`；不满足则**启动直接失败** |
| `SERVER_TRUSTED_PROXIES` | 空 | **反代部署必填** | 可信反向代理/LB 的来源 IP 或 CIDR，逗号分隔（如 `10.0.0.0/8,192.168.1.10`）。为空时不信任任何代理：`X-Forwarded-For` 一律忽略，IP 限流与访问日志只认 TCP 对端地址。Gin 出厂默认信任所有代理，客户端因此可伪造 XFF 随意换限流桶，本项目启动时按本配置收紧 |

---

## PostgreSQL

**方式一：完整 DSN（优先）**

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `DATABASE_URL` | — | 是 | 完整连接串，如 `postgres://user:pass@host:5432/db?sslmode=disable`；生产环境必须显式携带 `sslmode=require/verify-ca/verify-full`，启动时强制校验（缺省时 pgx 按 `prefer` 处理，会静默回落明文连接） |

**方式二：分项配置（`DATABASE_URL` 为空时生效）**

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `DB_HOST` | `localhost` | 否 | 主机 |
| `DB_PORT` | `5432` | 否 | 端口 |
| `DB_USER` | `postgres` | 否 | 用户名 |
| `DB_PASSWORD` | — | 否 | 密码 |
| `DB_NAME` | `app` | 否 | 数据库名 |
| `DB_SSL_MODE` | `disable` | 否 | `disable` / `require` / `verify-full` |
| `DB_MAX_CONNS` | `20` | 否 | 最大连接数 |
| `DB_MIN_CONNS` | `2` | 否 | 最小空闲连接数 |
| `DB_MAX_CONN_IDLE` | `10m` | 否 | 空闲连接最大存活时间 |
| `DB_MAX_CONN_LIFE` | `1h` | 否 | 连接最大存活时间 |
| `DB_STATEMENT_TIMEOUT` | `3s` | 否 | PostgreSQL `statement_timeout`（连接建立时下发），防止慢查询长期占用连接池；`0` 表示不设置。MySQL 驱动下当前不生效 |

---

## Redis

**方式一：完整 URL（优先）**

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `REDIS_URL` | — | 是 | 完整连接串，如 `redis://:pass@host:6379/0` |

**方式二：分项配置（`REDIS_URL` 为空时生效）**

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `REDIS_HOST` | `localhost` | 否 | 主机 |
| `REDIS_PORT` | `6379` | 否 | 端口 |
| `REDIS_PASSWORD` | — | 否 | 密码 |
| `REDIS_DB` | `0` | 否 | 数据库编号 |
| `REDIS_POOL_SIZE` | `10` | 否 | 连接池大小 |
| `REDIS_READ_TIMEOUT` | `3s` | 是 | 单次 Redis 读超时，必须大于 0 |
| `REDIS_WRITE_TIMEOUT` | `3s` | 是 | 单次 Redis 写超时，必须大于 0 |
| `REDIS_KEY_PREFIX` | `app` | 是 | 业务键前缀，多项目共用 Redis 时避免冲突；不能为空，也不能含冒号或空白 |

---

## JWT

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `JWT_SECRET` | `dev-secret-change-in-production` | **生产必填** | 签名密钥，生产环境务必使用强随机字符串 |
| `JWT_ACCESS_TTL` | `30m` | 否 | Access Token 有效期。必须小于 `JWT_REFRESH_TTL`；改密/降权后的权限回收最坏多等这么久 |
| `JWT_REFRESH_TTL` | `168h` | 否 | Refresh Token 有效期（7 天），同时也是 session 的**绝对**寿命（刷新不续期，见 session.md） |

---

## Auth

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `AUTH_MOCK_CODE_ENABLED` | `true` | 否 | 开发环境固定验证码 `123456`，**生产必须为 `false`** |
| `AUTH_MULTI_DEVICE_ENABLED` | `true` | 否 | `true`=多设备同时在线；`false`=单设备（新登录踢掉旧会话） |
| `AUTH_REDIS_FAIL_OPEN` | `true`（生产 `false`） | 否 | Redis 故障时鉴权策略：`true`=放行（可用性优先）；`false`=拒绝（安全性优先）。生产环境默认 fail-closed |
| `AUTH_REQUIRE_CODE_FOR_PASSWORD_REGISTER` | `true` | 否 | 密码注册（`grant_type=password`）是否必须先通过一次性验证码，证明持有该手机号/邮箱。`true`（默认，生产强制）：注册即归属证明，同时消除 `/register` 的账号存在性枚举 oracle；`false`：回到「只凭账号+密码就能建号」的旧行为，号码可被抢注，仅适合不对公网开放注册的部署，**production 启动校验会直接拒绝** |
| `AUTH_CODE_CHANNELS` | 空（自动） | 否 | 允许下发验证码的渠道白名单，逗号分隔（`sms,email`）。空=自动按已配置的 Provider 推导：只接 SMTP 时自动禁用 `sms`，请求短信得到 400 而不是运行期 500。显式配置时每个渠道都必须有 Provider，否则启动失败 |
| `AUTH_CODE_DAILY_LIMIT_PER_TARGET` | `10` | 否 | 同一手机号/邮箱在滚动 24 小时内的下发上限（成本保护）。`<=0` 关闭该维度，**production 启动校验要求 > 0** |
| `AUTH_CODE_DAILY_LIMIT_PER_IP` | `100` | 否 | 同一客户端 IP 在滚动 24 小时内的下发上限（所有渠道共用）。`<=0` 关闭该维度，**production 启动校验要求 > 0** |

---

## SMTP（验证码邮件下发，可选）

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `SMTP_HOST` | — | 否 | SMTP 服务器地址，为空则不启用邮件下发 |
| `SMTP_PORT` | `587` | 否 | SMTP 端口 |
| `SMTP_USERNAME` | — | 否 | 用户名 |
| `SMTP_PASSWORD` | — | 否 | 密码 |
| `SMTP_FROM` | `no-reply@example.com` | 否 | 发件人地址 |
| `SMTP_DIAL_TIMEOUT` | `5s` | 否 | 建立 TCP + TLS 连接的预算。配置了 `SMTP_HOST` 时必须 > 0，且与 `SMTP_SEND_TIMEOUT` 之和小于 `SERVER_SEND_CODE_REQUEST_TIMEOUT` |
| `SMTP_SEND_TIMEOUT` | `10s` | 否 | 从 EHLO 到 `QUIT` 的预算。配置了 `SMTP_HOST` 时必须 > 0；实际取 `min(该值, 发码请求剩余预算)`，与请求预算**共享而非相加** |

---

## OAuth

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `OAUTH_DEV_MODE` | `true` | 否 | 开发模式：直接用 `provider + open_id` 登录，**生产必须为 `false`** |
| `OAUTH_PROVIDERS` | `wechat,apple,google` | 否 | 启用的第三方登录平台，逗号分隔 |
| `OAUTH_WECHAT_APP_ID` | — | **生产必填**（启用 wechat 时） | 微信移动应用 AppID |
| `OAUTH_WECHAT_APP_SECRET` | — | 否 | 微信 AppSecret（预留） |
| `OAUTH_APPLE_CLIENT_ID` | — | **生产必填**（启用 apple 时） | Sign in with Apple 的 Services ID / Bundle ID |
| `OAUTH_GOOGLE_CLIENT_ID` | — | **生产必填**（启用 google 时） | Google OAuth Client ID（校验 `id_token` 的 `aud`） |

---

## 日志

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `LOG_LEVEL` | `info` | 否 | `debug` / `info` / `warn` / `error` |
| `LOG_FORMAT` | `console` | 否 | `console`（彩色）/ `json`（生产推荐） |

---

## 限流

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `RATE_LIMIT_PER_MINUTE` | `120` | 否 | 未登录/公开路由：每 IP 每分钟最大请求数 |
| `RATE_LIMIT_USER_PER_MINUTE` | `120` | 否 | 已鉴权 `/user`、`/admin`：每 user_id 每分钟最大请求数 |
| `RATE_LIMIT_AUTH_PER_MINUTE` | `20` | 否 | `/auth/login`、`/auth/register`：每 IP 每分钟最大请求数，**独立于公共桶**（两者共用一个桶）。每次请求都要做一次 bcrypt（cost 12，约 200–400ms），因此额度远低于普通接口；`<=0` 时也用默认 20。客户端 IP 取值遵循 `SERVER_TRUSTED_PROXIES` |

---

## Metrics

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `METRICS_ENABLED` | `true` | 否 | 暴露 Prometheus `/metrics` 端点 |
| `METRICS_TOKEN` | — | **生产必填**（`METRICS_ENABLED=true` 时） | `/metrics` 端点 bearer token，防止绕过 Ingress 直连。仅支持 `Authorization: Bearer` 头（Prometheus `scrape_config` 的 `bearer_token`），不支持 query 参数 |

---

## CORS

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `CORS_ALLOW_ORIGINS` | `http://localhost:5173,http://localhost:3000` | 否 | 允许的跨域来源，逗号分隔 |
| `CORS_ALLOW_CREDENTIALS` | `true` | 否 | 是否允许携带 Cookie；为 `true` 时 `CORS_ALLOW_ORIGINS` 不能包含 `*`（浏览器规范不允许，启动时校验会拒绝） |

---

## 生产环境必改清单

| 变量 | 原因 |
|------|------|
| `JWT_SECRET` | 默认值是公开的开发密钥，必须替换为强随机字符串（≥32 字节） |
| `AUTH_MOCK_CODE_ENABLED` | 必须为 `false`，否则任何人可用 `123456` 登录 |
| `AUTH_REQUIRE_CODE_FOR_PASSWORD_REGISTER` | 必须保持 `true`（默认）：关掉后密码注册不校验手机号/邮箱归属，攻击者可抢注他人号码，`/register` 还会变成账号存在性查询接口；production 启动校验会拒绝 `false` |
| `AUTH_CODE_DAILY_LIMIT_PER_TARGET` / `..._PER_IP` | 必须 > 0（默认 10 / 100）：这是短信/邮件成本保护；关闭后单 IP 每分钟可对 60 个不同号码发码 |
| `AUTH_CODE_CHANNELS` | 可选。留空时按已有 Provider 自动推导；显式写 `sms` 但未接入短信 Provider 会导致启动失败 |
| `OAUTH_DEV_MODE` | 必须为 `false`，否则可绕过第三方 token 校验 |
| `OAUTH_GOOGLE_CLIENT_ID` | 启用 google 登录时必填；未配置时 google 登录直接拒绝（`ErrNotConfigured`），生产启动时强制校验 |
| `OAUTH_APPLE_CLIENT_ID` | 启用 apple 登录时必填 |
| `OAUTH_WECHAT_APP_ID` | 启用 wechat 登录时必填 |
| `METRICS_TOKEN` | 启用 metrics 时必填，防止绕过 Ingress 直连暴露指标 |
| `AUTH_REDIS_FAIL_OPEN` | 生产默认 `false`（安全优先）；需可用性优先可设为 `true` |
| `DATABASE_URL` / `DB_PASSWORD` | 使用真实数据库凭据；DSN 必须显式携带 `sslmode=require/verify-ca/verify-full`（启动强制校验） |
| `REDIS_URL` / `REDIS_PASSWORD` | 使用真实 Redis 凭据 |
| `CORS_ALLOW_ORIGINS` | 配置为实际前端域名，不要用 `*` |
| `SERVER_TRUSTED_PROXIES` | 服务在 nginx / SLB / Ingress 之后时必须填代理网段：留空则所有用户共用代理那一个 IP 限流桶；填 `0.0.0.0/0`（等于 Gin 默认）则任何人都能伪造 XFF 绕过限流 |
