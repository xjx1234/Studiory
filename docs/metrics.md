# 指标与可观测（Prometheus）

应用在 `/metrics` 暴露 Prometheus 文本格式指标。应用只输出**原始指标**（计数器 / 直方图 / Gauge），
QPS、P99、错误率等由 Prometheus / Grafana 用 PromQL 派生。

## 开关与安全

由 `metrics.enabled` 控制（环境变量 `METRICS_ENABLED`），默认 `true`。

```yaml
metrics:
  enabled: true
  token: ""                   # /metrics bearer token，生产环境必须配置
```

- 关闭后不注册中间件，也不挂载 `/metrics` 端点。
- **应用层保护**：`metrics.token` 非空时，`/metrics` 端点要求 `Authorization: Bearer <token>` 才能访问（常数时间比较）。对应 Prometheus `scrape_config` 的 `bearer_token` 字段；**不支持** `?token=` query 参数（会随访问日志落盘）。
- **生产环境强制**：`config.Validate()` 在生产模式下要求 `metrics.enabled=true` 时必须配置 `metrics.token`，防止绕过 Ingress 直连暴露指标。
- 开发环境 `token` 为空时 `/metrics` 无鉴权，方便本地调试。
- 网络层（Ingress `server-snippet` 返回 403）与应用层 bearer token 构成双重防护。

## 暴露的指标

### HTTP

| 指标 | 类型 | label | 说明 |
|------|------|-------|------|
| `http_requests_total` | counter | `method`, `route`, `status` | 请求总数 |
| `http_request_duration_seconds` | histogram | `method`, `route`, `status` | 请求耗时分布 |
| `http_requests_in_flight` | gauge | — | 当前在途请求数 |

> `route` 用**路由模板**（如 `/api/v1/user/todos/:id`）而非真实路径，未匹配路由统一记为 `unmatched`，避免 label 基数爆炸。

### 验证码下发

| 指标 | 类型 | label | 说明 |
|------|------|-------|------|
| `auth_code_send_total` | counter | `channel`, `outcome` | 验证码下发次数 |
| `auth_code_cleanup_total` | counter | `channel`, `result` | 下发失败后清理冷却/验证码键的次数 |

- `channel`：`sms` / `email` / `unknown`（请求体里的其它值统一归为 `unknown`，避免标签基数无上限）。
- `outcome`：`sent` / `cooldown` / `target_quota` / `ip_quota` / `unsupported_channel` /
  `invalid_request` / `storage_error` / `dispatch_error`。
- `result`：`ok` / `error`。清理走独立于请求的 ctx（`context.WithoutCancel` + 独立超时），
  慢 SMTP 把请求预算耗尽后仍能真的删掉键。`error` 持续非 0 意味着用户看不到邮件、重试又被 429，
  同时日配额已在下发前扣掉（配额按请求计数，不因下发失败退还）。
- 只在 `metrics.enabled=true` 时装配；关闭指标时会传 nil observer，业务逻辑不受影响。

### 限流

| 指标 | 类型 | label | 说明 |
|------|------|-------|------|
| `rate_limit_store_fallback_total` | counter | `scope` | 限流存储从 Redis 降级为进程内内存的次数 |

- `scope`：`ip`（公开路由按客户端 IP）/ `uid`（已鉴权路由按 user_id）。
- 降级在 **store 初始化（进程启动）时判定一次**，不是每次请求累加：单进程内每个 `scope` 最多 +1，
  所以这条指标读的是「有没有发生过」，不要用 rate() 看趋势。
- 非 0 表示限流计数只在单个副本内生效：**多副本部署下实际阈值≈配置值 × 副本数**，
  429 会比预期少得多。降级同时打 `logger.Error`，因此关掉指标（`metrics.enabled=false`）也看得见。

### PostgreSQL 连接池（pgxpool）

| 指标 | 类型 | 说明 |
|------|------|------|
| `pgxpool_total_conns` | gauge | 当前连接总数 |
| `pgxpool_idle_conns` | gauge | 空闲连接数 |
| `pgxpool_acquired_conns` | gauge | 已占用连接数 |
| `pgxpool_max_conns` | gauge | 最大连接数 |
| `pgxpool_acquire_total` | counter | 累计获取连接次数 |

### Redis 连接池（go-redis）

| 指标 | 类型 | 说明 |
|------|------|------|
| `redis_pool_total_conns` | gauge | 当前连接总数 |
| `redis_pool_idle_conns` | gauge | 空闲连接数 |
| `redis_pool_hits_total` | counter | 命中空闲连接次数 |
| `redis_pool_misses_total` | counter | 未命中（需新建）次数 |
| `redis_pool_timeouts_total` | counter | 等待连接超时次数 |
| `redis_pool_stale_conns` | counter | 移除的陈旧连接数 |

此外还包含 `client_golang` 默认的 Go runtime（goroutine / GC / 内存）与进程（CPU / FD）指标。

## 常用 PromQL

```promql
# QPS（每秒请求数，按路由）
sum by (route) (rate(http_requests_total[1m]))

# 错误率（5xx 占比）
sum(rate(http_requests_total{status=~"5.."}[5m]))
  / sum(rate(http_requests_total[5m]))

# P99 延迟（按路由）
histogram_quantile(0.99,
  sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))

# 平均延迟
sum(rate(http_request_duration_seconds_sum[5m]))
  / sum(rate(http_request_duration_seconds_count[5m]))

# 验证码下发结果分布（定位配额/冷却/下发失败）
sum by (channel, outcome) (rate(auth_code_send_total[5m]))

# PG 连接池占用率
pgxpool_acquired_conns / pgxpool_max_conns

# Redis 连接池未命中率
rate(redis_pool_misses_total[5m])
  / (rate(redis_pool_hits_total[5m]) + rate(redis_pool_misses_total[5m]))

# 下发失败占比（慢 SMTP / 网关故障的主信号，适合做告警；阈值按自身量级定）
sum(rate(auth_code_send_total{outcome="dispatch_error"}[10m]))
  / sum(rate(auth_code_send_total[10m]))

# 清理失败：持续非 0 表示用户除了收不到验证码，还要再被冷却键挡 60 秒，且日配额已白扣
sum by (channel) (rate(auth_code_cleanup_total{result="error"}[10m]))

# 限流降级（启动期判定，用 increase 而非 rate）
sum by (scope) (increase(rate_limit_store_fallback_total[1h])) > 0
```

> 配额按**请求**计数，下发失败不退还（滚动 24h 窗口，见 [code-sender.md](code-sender.md)）。
> 因此 `dispatch_error` 与 `auth_code_cleanup_total{result="error"}` 同时上升时，
> 单个目标可能在窗口内被自己的重试打进 `target_quota`，只看 5xx 率会低估影响面。

## 本地验证

```bash
make run
# 开发环境（token 为空）可直接访问
curl -s http://localhost:8080/metrics | head -40

# 生产环境（token 非空）需携带 bearer token
curl -s -H "Authorization: Bearer <METRICS_TOKEN>" http://localhost:8080/metrics | head -40
```
