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

---

## PostgreSQL

**方式一：完整 DSN（优先）**

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `DATABASE_URL` | — | 是 | 完整连接串，如 `postgres://user:pass@host:5432/db?sslmode=disable` |

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
| `REDIS_KEY_PREFIX` | `app` | 否 | 业务键前缀，多项目共用 Redis 时避免冲突 |

---

## JWT

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `JWT_SECRET` | `dev-secret-change-in-production` | **生产必填** | 签名密钥，生产环境务必使用强随机字符串 |
| `JWT_ACCESS_TTL` | `2h` | 否 | Access Token 有效期 |
| `JWT_REFRESH_TTL` | `168h` | 否 | Refresh Token 有效期（7 天） |

---

## Auth

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `AUTH_MOCK_CODE_ENABLED` | `true` | 否 | 开发环境固定验证码 `123456`，**生产必须为 `false`** |
| `AUTH_MULTI_DEVICE_ENABLED` | `true` | 否 | `true`=多设备同时在线；`false`=单设备（新登录踢掉旧会话） |
| `AUTH_REDIS_FAIL_OPEN` | `true`（生产 `false`） | 否 | Redis 故障时鉴权策略：`true`=放行（可用性优先）；`false`=拒绝（安全性优先）。生产环境默认 fail-closed |

---

## SMTP（验证码邮件下发，可选）

| 变量 | 默认值 | 必填 | 说明 |
|------|--------|------|------|
| `SMTP_HOST` | — | 否 | SMTP 服务器地址，为空则不启用邮件下发 |
| `SMTP_PORT` | `587` | 否 | SMTP 端口 |
| `SMTP_USERNAME` | — | 否 | 用户名 |
| `SMTP_PASSWORD` | — | 否 | 密码 |
| `SMTP_FROM` | `no-reply@example.com` | 否 | 发件人地址 |

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
| `CORS_ALLOW_CREDENTIALS` | `true` | 否 | 是否允许携带 Cookie |

---

## 生产环境必改清单

| 变量 | 原因 |
|------|------|
| `JWT_SECRET` | 默认值是公开的开发密钥，必须替换为强随机字符串（≥32 字节） |
| `AUTH_MOCK_CODE_ENABLED` | 必须为 `false`，否则任何人可用 `123456` 登录 |
| `OAUTH_DEV_MODE` | 必须为 `false`，否则可绕过第三方 token 校验 |
| `OAUTH_GOOGLE_CLIENT_ID` | 启用 google 登录时必填；未配置时 google 登录直接拒绝（`ErrNotConfigured`），生产启动时强制校验 |
| `OAUTH_APPLE_CLIENT_ID` | 启用 apple 登录时必填 |
| `OAUTH_WECHAT_APP_ID` | 启用 wechat 登录时必填 |
| `METRICS_TOKEN` | 启用 metrics 时必填，防止绕过 Ingress 直连暴露指标 |
| `AUTH_REDIS_FAIL_OPEN` | 生产默认 `false`（安全优先）；需可用性优先可设为 `true` |
| `DATABASE_URL` / `DB_PASSWORD` | 使用真实数据库凭据 |
| `REDIS_URL` / `REDIS_PASSWORD` | 使用真实 Redis 凭据 |
| `CORS_ALLOW_ORIGINS` | 配置为实际前端域名，不要用 `*` |
