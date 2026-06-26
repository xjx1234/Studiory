# Redis 键与用途说明

脚手架后端使用 Redis 做验证码、Token 黑名单等。默认键前缀为 `app:`，可通过 `REDIS_KEY_PREFIX` 或 `config/*.yaml` 的 `redis.key_prefix` 修改。

---

## 1. 验证码（登录/注册）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:sms:{phone}` | string | 5 分钟 | 手机号对应的短信验证码 |
| `app:email:{email}` | string | 5 分钟 | 邮箱对应的邮件验证码 |
| `app:sms:cooldown:{phone}` | string | 60 秒 | 短信发送冷却（同一号码 60 秒内不可重发） |
| `app:email:cooldown:{email}` | string | 60 秒 | 邮件发送冷却 |

- 发送验证码时：先 SETNX 冷却 key；通过后再生成 6 位数字，写入对应 key，SET 时带 EX 300。
- 登录校验时：GET 后与用户输入比对，一致则删除 key（一次有效）。

---

## 2. Access Token 用户级吊销（改密 / 禁用）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:revoke:uid:{user_id}` | string（Unix 时间戳） | 与 Access Token 有效期一致 | 改密或禁用账号后，吊销该用户此前签发的 access token |

- 写入当前时间戳；`Auth` 中间件校验 token 的 `iat` 是否早于该时间戳。
- **不再用于普通登出**（登出改为按 session 吊销，见第 7 节），避免多设备模式下误伤其他设备。
- 改密、后台禁用账号仍会写入该键，作为 access token 的兜底吊销。
- Redis 不可用时吊销检查 **fail-open**（放行并打 Warn 日志）。

---

## 7. 登录会话（多设备 / 单设备）

由 `auth.multi_device_enabled` 控制（环境变量 `AUTH_MULTI_DEVICE_ENABLED`）。

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:session:{session_id}` | string（user_id） | 与 Refresh Token 有效期一致 | 单个会话是否存在 |
| `app:sessions:uid:{user_id}` | SET（session_id…） | 与 Refresh Token 一致 | **多设备模式**：该用户全部有效 session |
| `app:active_session:uid:{user_id}` | string（session_id） | 与 Refresh Token 一致 | **单设备模式**：当前唯一有效 session |

- JWT 的 `sid` 声明与 Redis 会话一一对应。
- **多设备模式**（`multi_device_enabled=true`，默认）：每次登录新建 session，互不影响；登出仅 `Revoke` 当前 session。
- **单设备模式**（`multi_device_enabled=false`）：新登录前 `RevokeAll` 并写入新的 `active_session`，旧设备 token 立即失效。
- `Auth` 中间件除用户级 `revoke:uid` 外，会校验 `sid` 是否仍在 Redis 中有效。

---

## 3. JWT Refresh Token 黑名单（登出 / 刷新轮换）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:blacklist:refresh:{sha256_hex}` | string | 与 Refresh Token 有效期一致 | 已失效的 refresh token（登出或轮换后） |

- 键后缀为 refresh token 的 SHA-256 十六进制摘要，避免 key 过长与截断冲突。
- 登出时：将 token 写入黑名单。
- 刷新 token 时：先查黑名单；签发新 token 后，将旧 refresh token 加入黑名单（轮换）。

---

## 4. 登录暴力破解防护（密码登录）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:login:fail:{sha256_hex}` | string（计数器） | 10 分钟 | 账号维度的连续登录失败计数 |
| `app:login:lock:{sha256_hex}` | string | 15 分钟 | 账号被锁定标记，存在即拒绝密码登录 |

- 键后缀为「账号标识（手机号/邮箱/account，统一小写去空格）」的 SHA-256 十六进制摘要，不存明文账号。
- 仅作用于**密码登录**路径：失败（含「用户不存在」，防止时序枚举账号）时原子递增 `fail` 计数；达到 **5 次/10 分钟** 阈值时写入 `lock` 键锁定 **15 分钟**。计数与锁定写入由同一段 Redis Lua 脚本原子完成。
- 登录成功立即删除 `fail` 计数（`lock` 键自然过期）。
- Redis 不可用时 **fail-open**（放行并打 Warn 日志），避免缓存故障完全阻断登录。
- 阈值/窗口/锁定时长为 `service/auth/service.go` 中常量（`loginMaxFailAttempts`、`loginFailWindow`、`loginLockDuration`），命中锁定返回错误码 `10010`（`err_account_locked`，HTTP 429）。

---

## 5. 限流

使用 Redis 分布式 store（`ulule/limiter`）。键前缀由 `REDIS_KEY_PREFIX` 控制，按维度分 scope：

| 维度 | Redis 前缀 | 适用路由 | 配置项 |
|------|------------|----------|--------|
| 客户端 IP | `${REDIS_KEY_PREFIX}:limiter:ip*` | `/api/v1/auth/*`、`/health`、`/ready` 等（**不含** `/user`、`/admin`） | `rate_limit.per_minute` / `RATE_LIMIT_PER_MINUTE` |
| 用户 ID | `${REDIS_KEY_PREFIX}:limiter:uid*` | `/api/v1/user/*`、`/api/v1/admin/*`（Auth 之后按 JWT `uid`） | `rate_limit.user_per_minute` / `RATE_LIMIT_USER_PER_MINUTE` |

- 已鉴权路由跳过 IP 限流，避免同 NAT 下多用户共享配额；未配置 `user_per_minute` 时默认与 `per_minute` 相同。
- 库内部会在上述前缀下生成计数与时间窗口相关键（具体后缀随实现变化）。
- Redis 不可用时回退到进程内 memory store（多实例限流不一致）。

---

## 6. 业务扩展

按模块自行约定，建议继续带项目前缀，例如：

- `app:cache:{resource}:{id}` — 热点数据缓存

会话相关键见第 7 节，不再使用笼统的 `app:user:{user_id}:session` 约定。

---

## 环境变量

- `REDIS_URL`：例如 `redis://localhost:6379/0`
- `REDIS_KEY_PREFIX`：默认 `app`
