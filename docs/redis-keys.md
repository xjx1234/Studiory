# Redis 键与用途说明

脚手架后端使用 Redis 做验证码、Token 黑名单等。默认键前缀为 `app:`，可通过 `REDIS_KEY_PREFIX` 或 `config/*.yaml` 的 `redis.key_prefix` 修改。

---

## 1. 验证码（登录/注册）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:sms:{phone}` | string | 5 分钟 | 手机号对应的短信验证码 |
| `app:email:{email}` | string | 5 分钟 | 邮箱对应的邮件验证码 |

- 发送验证码时：生成 6 位数字，写入对应 key，SET 时带 EX 300。
- 登录校验时：GET 后与用户输入比对，一致则删除 key（一次有效）。

---

## 2. JWT Refresh Token 黑名单（登出）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:blacklist:refresh:{token}` | string | 与 Refresh Token 有效期一致 | 已登出的 refresh token |

- 登出时：将 token 写入黑名单。
- 刷新 token 时：先查黑名单，若存在则拒绝。

---

## 3. 限流

当前使用 Redis 分布式 store（`ulule/limiter`）。键前缀由 `REDIS_KEY_PREFIX` 控制：

- `${REDIS_KEY_PREFIX}:limiter*`：限流计数与时间窗口相关键（由库内部生成，具体后缀随实现变化）。

如果 Redis 不可用时会自动回退到进程内 memory store（此时不适合多实例限流一致性）。

---

## 4. 业务扩展

按模块自行约定，建议继续带项目前缀，例如：

- `app:cache:{resource}:{id}` — 热点数据缓存
- `app:user:{user_id}:session` — 单设备登录等

---

## 环境变量

- `REDIS_URL`：例如 `redis://localhost:6379/0`
- `REDIS_KEY_PREFIX`：默认 `app`
