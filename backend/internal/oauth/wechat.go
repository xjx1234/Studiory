package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const wechatUserInfoURL = "https://api.weixin.qq.com/sns/userinfo"

// WechatConfig 微信开放平台 / 移动应用 OAuth 配置。
type WechatConfig struct {
	AppID string
}

// WechatProvider 通过微信 sns/userinfo 接口校验 access_token + open_id。
type WechatProvider struct {
	cfg         WechatConfig
	client      *http.Client
	userInfoURL string // 测试用覆盖；为空则使用默认生产地址
}

func NewWechatProvider(cfg WechatConfig) *WechatProvider {
	return &WechatProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *WechatProvider) Name() string { return ProviderWechat }

func (p *WechatProvider) Verify(ctx context.Context, req VerifyRequest) (*Identity, error) {
	accessToken := strings.TrimSpace(req.AccessToken)
	openID := strings.TrimSpace(req.OpenID)
	if accessToken == "" || openID == "" {
		return nil, fmt.Errorf("%w: wechat requires access_token and open_id", ErrInvalidToken)
	}

	q := url.Values{}
	q.Set("access_token", accessToken)
	q.Set("openid", openID)
	q.Set("lang", "zh_CN")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoEndpoint()+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var payload struct {
		OpenID     string `json:"openid"`
		Nickname   string `json:"nickname"`
		HeadImgURL string `json:"headimgurl"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: wechat response decode: %v", ErrInvalidToken, err)
	}
	if payload.ErrCode != 0 {
		return nil, fmt.Errorf("%w: wechat api %d %s", ErrInvalidToken, payload.ErrCode, payload.ErrMsg)
	}
	if payload.OpenID == "" || payload.OpenID != openID {
		return nil, ErrInvalidToken
	}

	return &Identity{
		OpenID:   payload.OpenID,
		Nickname: payload.Nickname,
		Avatar:   payload.HeadImgURL,
	}, nil
}

func (p *WechatProvider) userInfoEndpoint() string {
	if p.userInfoURL != "" {
		return p.userInfoURL
	}
	return wechatUserInfoURL
}
