# 验证码下发（多服务商）

验证码下发抽象在 `internal/sender`，业务层（auth service）只依赖 `Sender` 接口，
具体服务商可自由替换、组合，并支持**同一渠道接入多家运营商做故障转移**。

## 核心概念

| 类型 | 作用 |
|------|------|
| `Channel` | 渠道：`sms` / `email` |
| `Message` | 一条下发请求（渠道 + 目标 + 验证码） |
| `Provider` | 单个服务商（阿里云短信、腾讯云短信、SMTP 邮件……），声明 `Supports(channel)` |
| `Router` | 按渠道路由；同渠道多 Provider 按注册顺序**故障转移** |
| `Sender` | 对外统一入口，业务层依赖它 |

## 下发流程（SendCode）

```
目标日配额 → IP 日配额 → 60s 冷却（同一段 Lua 原子判定）
  → 生成验证码 → 写 Redis(5min) → Router.Send → 选渠道 Provider 依次尝试
```

- 配额与冷却：滚动 24 小时按目标（默认 10 次）与按 IP（默认 100 次）计数，超出返回 429；
  冷却中的请求不消耗配额。计数与判定在同一个 Lua 脚本里完成，并发请求不会同时穿透。
- 验证码生成：开发模式（`auth.mock_code_enabled=true`）用固定 `123456` 便于联调；否则生成 6 位 `crypto/rand` 随机码，**rand 失败时返回错误而非回退固定码**。
- 下发失败：清除冷却键与验证码，允许用户立即重试；配额计数保留，避免用「失败重试」绕过上限。
  清理**不复用请求 ctx**——走到这一步多半是请求预算已被慢网关耗尽，沿用已取消的 ctx 会让
  `DEL` 根本没发出去（冷却键继续挡用户 60 秒、验证码键再躺 5 分钟）。清理用
  `context.WithoutCancel(ctx)` 派生的独立短预算，结果记入 `auth_code_cleanup_total{channel,result}`，
  失败同时打内部日志，不允许 `_ =` 吞掉。
- 故障转移：同一渠道的多个 Provider 按注册顺序尝试，任一成功即返回；全部失败才报错。
  每次尝试只拿「剩余预算 ÷ 剩余尝试数」，否则主通道把请求预算拖光后备用通道只会拿到
  一个已过期的 ctx，故障转移形同虚设；上下文已结束时不再空转，并在错误里说明剩余通道未被尝试。
- 指标：`auth_code_send_total{channel,outcome}`，`channel` 仅 `sms` / `email` / `unknown`，
  `outcome` 为 `sent` / `cooldown` / `target_quota` / `ip_quota` /
  `unsupported_channel` / `invalid_request` / `storage_error` / `dispatch_error`；
  清理结果见 `auth_code_cleanup_total{channel,result}`（[metrics.md](metrics.md)）。

## 渠道配置（auth.code_channels）

- 留空（默认）：启动时按已接入的 Provider 自动推导。只配了 SMTP 时 `sms` 自动禁用，
  请求短信返回 `400`（提示「该验证码渠道暂不可用」），不会出现「启动正常、运行期 500」。
- 显式配置：`auth.code_channels: [email]` 等；列出的渠道必须有 Provider，否则**启动失败**（fail fast），
  避免把不可用的渠道暴露给调用方。

## 内置参考实现

| Provider | 渠道 | 说明 |
|----------|------|------|
| `MockProvider` | 全部 | 只打日志，不真实发送，开发/测试默认 |
| `SMTPProvider` | email | 基于标准库 `net/smtp`，带连接超时、整体 deadline 与 STARTTLS 约束 |

### SMTPProvider 的连接与 TLS 约束

`/api/v1/auth/send-code` 是同步调用 `Provider.Send`，所以 `net/smtp.SendMail` 那种「无超时」的写法会把请求 goroutine 一直挂在慢连接上。`SMTPProvider` 显式处理三件事：

- **建连超时**（默认 5s，`smtp.dial_timeout`，`net.Dialer.Timeout`）与**全流程超时**（默认 10s，
  `smtp.send_timeout`），并收敛到 `ctx` 更早的 deadline：`deliver` 取 `min(send_timeout, ctx 剩余)`，
  所以它与请求预算是**共享**而不是相加——预算比两者之和还小时，表现为每次都 503 而不是「慢一点」；
- **ctx 取消立即打断阻塞读写**：`context.AfterFunc` 在取消时把连接 deadline 拉到当前时刻；
- **有凭据时强制 STARTTLS**：`username` 非空但服务端未声明 STARTTLS 时直接报错，避免 `net/smtp` 明文提交邮箱密码。未配置凭据（内网中继）时允许明文，此时没有可泄漏的认证信息。

## 配置（SMTP，可选）

`smtp.host` 为空则不启用 SMTP，回退到 mock。

```yaml
smtp:
  host: ""                 # 如 smtp.example.com
  port: 587
  username: ""
  password: ""
  from: "no-reply@example.com"
  dial_timeout: 5s         # 建连预算
  send_timeout: 10s        # EHLO 到 QUIT 的整体预算

app:
  send_code_request_timeout: 20s   # 仅 /api/v1/auth/send-code；留空则沿用 app.request_timeout
```

对应环境变量：`SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM` /
`SMTP_DIAL_TIMEOUT` / `SMTP_SEND_TIMEOUT`，以及 `SERVER_SEND_CODE_REQUEST_TIMEOUT`。

`Validate()` 在启动阶段强制这条链（配了 `smtp.host` 才校验）：

```
smtp.dial_timeout + smtp.send_timeout < app.send_code_request_timeout ≤ app.write_timeout
```

不满足就拒绝启动：请求 ctx 先到期会把 SMTP 连接 deadline 拉到当下，读写立刻失败，
每次发码都以 503 收场——这类「外层小于内层」的错配不该等到线上才发现。
详见 [deploy.md](deploy.md) 的「HTTP Server 超时」。

## 接入一家短信运营商

1. 实现 `sender.Provider`：

```go
type AliyunSMS struct { /* client, signName, templateCode ... */ }

func (p *AliyunSMS) Name() string              { return "aliyun-sms" }
func (p *AliyunSMS) Supports(ch sender.Channel) bool { return ch == sender.ChannelSMS }
func (p *AliyunSMS) Send(ctx context.Context, msg sender.Message) error {
    // 调用阿里云短信 SDK，把 msg.Code 填入模板参数
    return nil
}
```

2. 在 `internal/app/app.go` 的 `buildCodeSender` 中追加到 `providers`：

```go
providers = append(providers,
    NewAliyunSMS(...),   // 主运营商（先注册，优先级高）
    NewTencentSMS(...),  // 备用运营商（主失败时自动切换）
)
```

`NewRouter` 会把同为 `sms` 渠道的多个 Provider 串成故障转移链：主运营商不可用时自动切到备用。

## 测试

- `internal/sender/sender_test.go`：渠道路由、故障转移、全部失败、无 Provider、mock 行为。
- `internal/service/auth/sendcode_test.go`：SendCode 生成并下发、随机码、冷却限频、目标/IP 日配额、渠道白名单、下发失败回滚、指标 outcome。
