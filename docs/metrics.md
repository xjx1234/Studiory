# 指标与可观测（Prometheus）

应用在 `/metrics` 暴露 Prometheus 文本格式指标。应用只输出**原始指标**（计数器 / 直方图 / Gauge），
QPS、P99、错误率等由 Prometheus / Grafana 用 PromQL 派生。

## 开关

由 `metrics.enabled` 控制（环境变量 `METRICS_ENABLED`），默认 `true`。

```yaml
metrics:
  enabled: true
```

- 关闭后不注册中间件，也不挂载 `/metrics` 端点。
- `/metrics` 默认不经过鉴权与限流。生产环境应在网络层（Ingress / 防火墙 / ServiceMonitor 内网）限制访问，或将 `enabled` 设为 `false` 并改用 sidecar。

## 暴露的指标

### HTTP

| 指标 | 类型 | label | 说明 |
|------|------|-------|------|
| `http_requests_total` | counter | `method`, `route`, `status` | 请求总数 |
| `http_request_duration_seconds` | histogram | `method`, `route`, `status` | 请求耗时分布 |
| `http_requests_in_flight` | gauge | — | 当前在途请求数 |

> `route` 用**路由模板**（如 `/api/v1/user/todos/:id`）而非真实路径，未匹配路由统一记为 `unmatched`，避免 label 基数爆炸。

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

# PG 连接池占用率
pgxpool_acquired_conns / pgxpool_max_conns

# Redis 连接池未命中率
rate(redis_pool_misses_total[5m])
  / (rate(redis_pool_hits_total[5m]) + rate(redis_pool_misses_total[5m]))
```

## 本地验证

```bash
make run
curl -s http://localhost:8080/metrics | head -40
```
