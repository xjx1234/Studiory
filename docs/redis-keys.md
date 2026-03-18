# Redis 键与用途说明

拾习社后端使用 Redis 做验证码、Token 黑名单、限流等，键统一带前缀 `shixi:`，便于区分与清理。

---

## 1. 验证码（登录/注册）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `shixi:sms:{phone}` | string | 5 分钟 | 手机号对应的短信验证码 |
| `shixi:email:{email}` | string | 5 分钟 | 邮箱对应的邮件验证码 |

- 发送验证码时：生成 6 位数字，写入对应 key，SET 时带 EX 300。
- 登录校验时：GET 后与用户输入比对，一致则删除 key（一次有效）。

---

## 2. JWT Refresh Token 黑名单（登出）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `shixi:blacklist:refresh:{jti}` | string | 与 Refresh Token 剩余有效期一致 | 已登出的 refresh token 的 jti（如用 jti 做 key） |

- 登出时：将当前 refresh token 的 jti 写入黑名单，TTL 设为该 token 的剩余有效秒数。
- 刷新 token 时：先查黑名单，若存在则拒绝。

若 JWT 未使用 jti，可用 `shixi:blacklist:refresh:{token_hash}` 存任意占位值，TTL 同上。

---

## 3. 限流（可选）

使用 `ulule/limiter` 的 Redis store 时，键由库自动管理，一般形式为：

| 键格式（示例） | 说明 |
|----------------|------|
| `limiter:{identifier}` | identifier 通常为 IP 或 user_id，具体以 limiter 文档为准 |

当前项目若仍用 memory store，可后续改为 Redis store，便于多实例部署时限流一致。

---

## 4. 其他可扩展键

- **热点缓存**：如 `shixi:word_set:{id}` 缓存单词集元信息，TTL 按需（如 10 分钟）。
- **用户会话/设备**：如需要“单设备登录”等，可增加 `shixi:user:{user_id}:session` 等。

---

## 环境变量

- `REDIS_URL`：连接串，例如 `redis://localhost:6379/0`（0 为 DB 编号）。
