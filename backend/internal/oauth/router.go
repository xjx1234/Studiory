package oauth

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Router 按 provider 路由到对应 Provider；devMode 时允许仅传 open_id 跳过远程校验。
type Router struct {
	byProvider map[string]Provider
	devMode    bool
	logger     *zap.Logger
}

// NewRouter 注册各平台 Provider。devMode 为 true 时，请求仅含 open_id 可直接通过（本地联调）。
func NewRouter(logger *zap.Logger, devMode bool, providers ...Provider) *Router {
	if logger == nil {
		logger = zap.NewNop()
	}
	r := &Router{
		byProvider: make(map[string]Provider),
		devMode:    devMode,
		logger:     logger,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		r.byProvider[strings.ToLower(p.Name())] = p
	}
	return r
}

// Verify 校验第三方登录凭证并返回用户标识。
func (r *Router) Verify(ctx context.Context, req VerifyRequest) (*Identity, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		return nil, fmt.Errorf("%w: empty provider", ErrInvalidToken)
	}

	openID := strings.TrimSpace(req.OpenID)
	accessToken := strings.TrimSpace(req.AccessToken)
	idToken := strings.TrimSpace(req.IDToken)

	// 开发模式捷径：仅传 open_id，不访问第三方 API（与 oauth.dev_mode 配置对应）。
	if r.devMode && openID != "" && accessToken == "" && idToken == "" {
		r.logger.Debug("oauth dev mode: accept open_id without remote verification",
			zap.String("provider", provider),
		)
		return &Identity{OpenID: openID}, nil
	}

	p, ok := r.byProvider[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoProvider, provider)
	}

	identity, err := p.Verify(ctx, VerifyRequest{
		Provider:    provider,
		AccessToken: accessToken,
		IDToken:     idToken,
		OpenID:      openID,
	})
	if err != nil {
		return nil, err
	}
	if identity == nil || strings.TrimSpace(identity.OpenID) == "" {
		return nil, ErrInvalidToken
	}
	identity.OpenID = strings.TrimSpace(identity.OpenID)
	return identity, nil
}
