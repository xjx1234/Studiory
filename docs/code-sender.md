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
冷却限频(SetNX) → 生成验证码 → 写 Redis(5min) → Router.Send → 选渠道 Provider 依次尝试
```

- 验证码生成：开发模式（`auth.mock_code_enabled=true`）用固定 `123456` 便于联调；否则生成 6 位 `crypto/rand` 随机码。
- 下发失败：清除冷却键与验证码，允许用户立即重试。
- 故障转移：同一渠道的多个 Provider 按注册顺序尝试，任一成功即返回；全部失败才报错。

## 内置参考实现

| Provider | 渠道 | 说明 |
|----------|------|------|
| `MockProvider` | 全部 | 只打日志，不真实发送，开发/测试默认 |
| `SMTPProvider` | email | 基于标准库 `net/smtp` 的可用邮件实现 |

## 配置（SMTP，可选）

`smtp.host` 为空则不启用 SMTP，回退到 mock。

```yaml
smtp:
  host: ""                 # 如 smtp.example.com
  port: 587
  username: ""
  password: ""
  from: "no-reply@example.com"
```

对应环境变量：`SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM`。

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
- `internal/service/auth/sendcode_test.go`：SendCode 生成并下发、随机码、冷却限频、下发失败回滚。
