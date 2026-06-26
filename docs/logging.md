# 结构化日志规范

应用使用 [Uber Zap](https://github.com/uber-go/zap) 输出结构化日志。生产环境建议 `log.format=json`（`LOG_FORMAT=json`），便于 ELK / Loki 等采集与检索。

## 配置

```yaml
log:
  level: info      # debug | info | warn | error
  format: console  # console（开发）| json（生产）
```

环境变量：`LOG_LEVEL`、`LOG_FORMAT`。初始化逻辑见 `backend/main.go` 的 `initLogger`。

## 标准字段

业务与 HTTP 日志统一使用 **snake_case** 字段名：

| 字段 | 类型 | 来源 | 说明 |
|------|------|------|------|
| `request_id` | string | `gin-contrib/requestid` | 单次 HTTP 请求唯一 ID；响应 JSON 与 `X-Request-Id` 头一致 |
| `user_id` | string | JWT `uid` / Auth 中间件 | 已鉴权请求的用户 UUID；未登录请求省略 |
| `method` | string | ginzap | HTTP 方法 |
| `path` | string | ginzap | 请求路径（不含 query） |
| `status` | int | ginzap | HTTP 状态码 |
| `latency` | duration | ginzap | 请求耗时 |
| `ip` | string | ginzap | 客户端 IP |
| `error` | string | zap.Error | 错误详情（仅 Error 级别） |

### 关联规则

1. **客户端可传 `X-Request-Id`**：若请求头已带合法 ID，服务端复用；否则自动生成。便于前后端、网关联调排障。
2. **JSON 响应体**的 `request_id` 与访问日志中的 `request_id` 相同（见 `pkg/resp`）。
3. **已鉴权路由**（`/api/v1/user/*`、`/api/v1/admin/*`）的访问日志自动附带 `user_id`。
4. **未鉴权路由**（`/api/v1/auth/*`、`/health` 等）仅有 `request_id`，不含 `user_id`。

## HTTP 访问日志

由 `gin-contrib/zap` 中间件输出，每条请求一行，在 handler 执行完毕后记录。

实现：`internal/http/middleware/access_log_fields.go` 通过 `GinzapWithConfig.Context` 注入 `request_id` / `user_id`。

开发环境（console）示例：

```text
{"level":"info","ts":"...","msg":"","status":200,"method":"GET","path":"/api/v1/user/profile","request_id":"a1b2c3...","user_id":"550e8400-e29b-41d4-a716-446655440000","latency":"2.1ms"}
```

生产环境（json）字段相同，便于按 `request_id` 或 `user_id` 过滤。

### 跳过的路径

默认记录所有路由。若需减少噪音（如高频健康检查），可在 `router.go` 的 `GinzapWithConfig` 中配置 `SkipPaths`（当前未跳过 `/health`）。

## Service 层内部错误

`internal/service/logging.go` 提供 `LogInternal`，各 service 嵌入 `LogSupport` 后调用：

```go
s.LogInternal("ChangePassword update password", err,
    zap.String("user_id", userID),
)
```

约定：

- **已知 `user_id` 时必须追加**，便于与访问日志串联。
- `op` 使用简短英文短语，描述失败步骤（与现有代码风格一致）。
- 不要记录密码、token、验证码等敏感值。

## 基础设施日志

启动、连接池、关机等由 `main.go`、`internal/store`、`internal/app` 直接打日志，使用语义化 `msg` + 结构化字段（如 `env`、`addr`、`version`）。这类日志无 `request_id`，属进程级事件。

## 与可观测性的关系

| 能力 | 文档 |
|------|------|
| Prometheus 指标 | [metrics.md](metrics.md) |
| 分布式追踪（OpenTelemetry） | 尚未接入，后续 trace_id 可与 `request_id` 并存 |

排障建议：先用响应中的 `request_id` 在日志系统检索完整请求链，再按 `user_id` 查看该用户近期行为。

## 涉及文件

| 层级 | 路径 |
|------|------|
| Logger 初始化 | `backend/main.go` |
| Request ID 中间件 | `internal/http/middleware/request_id.go` |
| 访问日志字段 | `internal/http/middleware/access_log_fields.go` |
| 路由挂载 | `internal/http/router.go` |
| 统一响应 request_id | `pkg/resp/response.go` |
| Service 错误日志 | `internal/service/logging.go` |
