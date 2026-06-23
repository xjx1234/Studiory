# 第三方 OAuth 登录

脚手架通过 `internal/oauth` 包抽象各平台 token 校验，业务层（`auth service`）只依赖 `oauth.Verifier` 接口。

## 核心概念

| 类型 | 说明 |
|------|------|
| `Provider` | 单个平台实现（微信 / Apple / Google），负责远程校验 token 并返回 `Identity` |
| `Verifier` | 对外统一入口，业务层注入 |
| `Router` | 按 `provider` 路由到对应 `Provider`；`dev_mode=true` 时允许仅传 `open_id` 跳过远程校验 |
| `Identity` | 校验结果：`open_id`、可选 `nickname` / `email` / `avatar` |

## 登录流程

```
客户端                          auth service                    oauth.Router
  │ POST /auth/login (grant_type=oauth)                              │
  │ provider + access_token/id_token  ──►  loginWithOAuth()          │
  │                                      resolveOAuthIdentity() ──► Verify()
  │                                                                   │
  │                                      ◄── Identity{OpenID,...} ◄──┘
  │                                      查/建 user_oauth 绑定 → 签发 JWT
```

1. 客户端携带 `grant_type=oauth` 与平台凭证调用 `POST /api/v1/auth/login`
2. `Router.Verify` 按平台校验 token（或 dev 模式直传 `open_id`）
3. 用解析出的 `open_id` 查 `user_oauth` 绑定；首次登录自动建用户并绑定
4. 签发 access / refresh token

## 各平台凭证约定

| 平台 | 生产环境请求字段 | 校验方式 |
|------|------------------|----------|
| wechat | `access_token` + `open_id` | 调用微信 `sns/userinfo` |
| google | `id_token` 或 `access_token` | 调用 Google `tokeninfo`（可校验 `aud`） |
| apple | `id_token` | Apple JWKS 验签 + `aud` 校验 |
| 开发模式 | 仅 `open_id`（`oauth.dev_mode=true`） | 跳过远程 API |

### 开发模式示例

```json
POST /api/v1/auth/login
{
  "grant_type": "oauth",
  "provider": "wechat",
  "open_id": "wx_openid_dev_001"
}
```

### 生产模式示例（Google）

```json
POST /api/v1/auth/login
{
  "grant_type": "oauth",
  "provider": "google",
  "id_token": "<Google Sign-In 返回的 JWT>"
}
```

## 内置实现

| Provider | 文件 | 说明 |
|----------|------|------|
| Wechat | `internal/oauth/wechat.go` | `sns/userinfo` 校验 |
| Google | `internal/oauth/google.go` | `oauth2.googleapis.com/tokeninfo` |
| Apple | `internal/oauth/apple.go` | 拉取 Apple JWKS 验签 `id_token` |

## 配置

`config/base.yaml`：

```yaml
oauth:
  dev_mode: true
  providers:
    - wechat
    - apple
    - google
  wechat:
    app_id: ""
    app_secret: ""      # 预留：code 换 token 流程
  apple:
    client_id: ""       # Sign in with Apple Services ID
  google:
    client_id: ""       # 校验 id_token 的 aud
```

环境变量：`OAUTH_DEV_MODE`、`OAUTH_WECHAT_APP_ID`、`OAUTH_APPLE_CLIENT_ID`、`OAUTH_GOOGLE_CLIENT_ID` 等。

生产环境 `config.Validate()` 会拒绝 `oauth.dev_mode=true`。

## 装配位置

`internal/app/app.go` 的 `buildOAuthVerifier()`：

```go
providers := []oauth.Provider{
    oauth.NewWechatProvider(oauth.WechatConfig{AppID: cfg.OAuthWechatAppID}),
    oauth.NewAppleProvider(oauth.AppleConfig{ClientID: cfg.OAuthAppleClientID}),
    oauth.NewGoogleProvider(oauth.GoogleConfig{ClientID: cfg.OAuthGoogleClientID}),
}
return oauth.NewRouter(logger, cfg.OAuthDevMode, providers...)
```

## 接入新平台

1. 在 `internal/oauth/` 新增 `xxx.go`，实现 `Provider` 接口
2. 在 `buildOAuthVerifier` 中 `append` 新 Provider
3. 在 `config` / `oauth.providers` / HTTP `LoginReq` 的 `oneof` 中注册平台名
4. 补充单元测试（推荐用 `httptest` mock 远程 API）

## 安全说明

- **dev_mode 仅用于本地联调**，生产必须关闭并由各 Provider 校验真实 token
- Apple Provider 未配置 `client_id` 时返回 `ErrNotConfigured`
- token 无效统一映射为 `10002 err_invalid_token`
- 首次 OAuth 登录自动创建 `role=user` 账号，昵称优先使用平台返回的 `nickname`
