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

## 技术实现

1. 每次登录生成 `session_id`（UUID），写入 JWT 的 `sid` 声明（access + refresh 均携带）。
2. Redis 记录会话状态，详见 [redis-keys.md](redis-keys.md) 第 7 节。
3. `Auth` 中间件校验：`sid` 在 Redis 中仍有效 **且** 未命中用户级 `revoke:uid`。
4. `Refresh` 保持同一 `sid` 轮换 token，不新建 session。
5. `Logout` 仅吊销当前 `sid` 对应 session（多设备下不影响其他设备）。

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
