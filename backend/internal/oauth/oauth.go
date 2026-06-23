// Package oauth 提供第三方登录 token 校验抽象与多平台路由。
//
// 设计：
//   - Provider 是单个 OAuth 平台（微信 / Apple / Google 等）的 token 校验实现。
//   - Router 按 provider 名称路由到对应 Provider；dev_mode 下允许客户端直传 open_id 跳过校验。
//   - 业务层（auth service）只依赖 Verifier 接口，不关心各平台 HTTP/JWKS 细节。
package oauth

import (
	"context"
	"errors"
)

// 平台名称常量（与 config oauth.providers / 登录请求 provider 字段保持一致）。
const (
	ProviderWechat = "wechat"
	ProviderApple  = "apple"
	ProviderGoogle = "google"
)

// AllProviders 列出脚手架内置的平台标识。
var AllProviders = []string{ProviderWechat, ProviderApple, ProviderGoogle}

var (
	// ErrNoProvider 表示未注册该平台的 Provider。
	ErrNoProvider = errors.New("oauth: no provider registered")
	// ErrInvalidToken 表示第三方 token 无效或已过期。
	ErrInvalidToken = errors.New("oauth: invalid token")
	// ErrNotConfigured 表示该平台尚未配置生产所需参数（如 client_id）。
	ErrNotConfigured = errors.New("oauth: provider not configured")
)

// Identity 是第三方 token 校验通过后解析出的用户标识。
type Identity struct {
	OpenID   string
	Nickname string
	Email    string
	Avatar   string
}

// VerifyRequest 是 token 校验入参。
//
// 生产环境：按平台传 access_token 和/或 id_token（微信另需 open_id 做 userinfo 校验）。
// 开发模式（Router.devMode=true）：可仅传 open_id 跳过远程校验。
type VerifyRequest struct {
	Provider    string
	AccessToken string
	IDToken     string
	OpenID      string
}

// Provider 是单个 OAuth 平台的 token 校验实现。
type Provider interface {
	// Name 返回平台标识（wechat / apple / google）。
	Name() string
	// Verify 校验第三方 token 并返回用户 open_id（及各平台可选资料）。
	Verify(ctx context.Context, req VerifyRequest) (*Identity, error)
}

// Verifier 是对外统一入口，业务层依赖它。
type Verifier interface {
	Verify(ctx context.Context, req VerifyRequest) (*Identity, error)
}
