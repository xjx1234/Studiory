# 多设备 / 单设备 Session 管理

通过配置 `auth.multi_device_enabled` 控制同一账号是否允许多设备同时在线。

## 配置

```yaml
auth:
  multi_device_enabled: true   # 默认：多设备
  # multi_device_enabled: false  # 单设备：新登录踢掉旧会话
```

环境变量：`AUTH_MULTI_DEVICE_ENABLED=true|false`

## 行为对比

| 场景 | 多设备（`true`） | 单设备（`false`） |
|------|------------------|-------------------|
| 用户 A 在手机登录 | 创建 session-1 | 创建 session-1 |
| 同一用户在电脑再登录 | 创建 session-2，两台都有效 | 创建 session-2，**session-1 立即失效** |
| 手机登出 | 仅吊销 session-1 | 吊销 session-1 |
| 改密 / 被禁用 | 吊销**全部** session + 用户级 access revoke | 同左 |

## 有效期语义（绝对过期）

Session 采用**绝对过期**，不是滑动过期：

- session 的 TTL 在登录时写入（`{prefix}:session:{sid}`），时长等于 `jwt.refresh_ttl`（默认 168h）；
- `Refresh` 只轮换 token，**不延长** session 的 TTL，也不重建 session；
- 因此用户最长在线时间 = 登录时刻 + `refresh_ttl`，到期后必须重新登录，即使手里还有一个“看起来很新”的 refresh token。

这样设计是为了避免 refresh token 变成“可无限续命的永久凭证”。如果将来需要滑动过期，必须同时续期
`session:{sid}` 与 `sessions:uid:{user_id}`，并更新本节与 [redis-keys.md](redis-keys.md) 第 7 节，不能只改一处。

`Auth` 中间件判断会话是否有效时**只看 `session:{sid}` 键**：键存在且 value 与当前 userID 一致即通过。
`sessions:uid` 集合只是 `RevokeAll` 的枚举索引，后续登录会顺带续期集合，但集合续期不会让已过期的旧 sid 复活。

## 技术实现

1. 每次登录生成 `session_id`（UUID），写入 JWT 的 `sid` 声明（access + refresh 均携带）。
2. Redis 记录会话状态，详见 [redis-keys.md](redis-keys.md) 第 7 节。
3. `Auth` 中间件校验：`sid` 在 Redis 中仍有效 **且** 未命中用户级 `revoke:uid`。
   两项检查合并为一次 MGET（`session.Store.CheckAccess`），每个已鉴权请求只需 1 次 Redis 往返。
4. Redis 故障时按 `auth.redis_fail_open` 策略降级（默认开发 `true`=放行，生产 `false`=拒绝）。
5. `Refresh` 保持同一 `sid` 轮换 token，不新建 session，也不续期 session（绝对过期，见上节）。
6. `Logout` 仅吊销当前 `sid` 对应 session（多设备下不影响其他设备）。

## 涉及文件

| 层级 | 路径 |
|------|------|
| 会话存储 | `internal/session/session.go` |
| JWT | `internal/auth/token.go`（`Claims.SessionID`） |
| 登录 | `internal/service/auth/login.go` |
| 刷新/登出 | `internal/service/auth/token.go` |
| 改密 | `internal/service/user/service.go` |
| 禁用用户 | `internal/service/admin/service.go` |
| 中间件 | `internal/http/middleware/auth.go` |
| 装配 | `internal/app/app.go` |

## 本地验证

```bash
# 单设备模式（docker-compose 或 local.yaml）
AUTH_MULTI_DEVICE_ENABLED=false

# 1. 用户登录拿 token-A
# 2. 同一账号再次登录拿 token-B
# 3. 用 token-A 调 /api/v1/user/profile → 401
# 4. 用 token-B 调 /api/v1/user/profile → 200
```
