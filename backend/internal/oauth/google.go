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

const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

// GoogleConfig Google OAuth 客户端配置。
type GoogleConfig struct {
	ClientID string // 非空时校验 token 的 aud 字段
}

// GoogleProvider 通过 Google tokeninfo 接口校验 id_token 或 access_token。
type GoogleProvider struct {
	cfg          GoogleConfig
	client       *http.Client
	tokenInfoURL string // 测试用覆盖；为空则使用默认生产地址
}

func NewGoogleProvider(cfg GoogleConfig) *GoogleProvider {
	return &GoogleProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *GoogleProvider) Name() string { return ProviderGoogle }

func (p *GoogleProvider) Verify(ctx context.Context, req VerifyRequest) (*Identity, error) {
	token := strings.TrimSpace(req.IDToken)
	param := "id_token"
	if token == "" {
		token = strings.TrimSpace(req.AccessToken)
		param = "access_token"
	}
	if token == "" {
		return nil, fmt.Errorf("%w: google requires id_token or access_token", ErrInvalidToken)
	}

	q := url.Values{}
	q.Set(param, token)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.tokenInfoEndpoint()+"?"+q.Encode(), nil)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: google tokeninfo status %d", ErrInvalidToken, resp.StatusCode)
	}

	var payload struct {
		Sub       string `json:"sub"`
		Aud       string `json:"aud"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		Picture   string `json:"picture"`
		Error     string `json:"error"`
		ErrorDesc string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: google response decode: %v", ErrInvalidToken, err)
	}
	if payload.Error != "" || payload.Sub == "" {
		return nil, fmt.Errorf("%w: google %s %s", ErrInvalidToken, payload.Error, payload.ErrorDesc)
	}

	clientID := strings.TrimSpace(p.cfg.ClientID)
	if clientID != "" && payload.Aud != "" && payload.Aud != clientID {
		return nil, fmt.Errorf("%w: google aud mismatch", ErrInvalidToken)
	}

	return &Identity{
		OpenID:   payload.Sub,
		Email:    payload.Email,
		Nickname: payload.Name,
		Avatar:   payload.Picture,
	}, nil
}

func (p *GoogleProvider) tokenInfoEndpoint() string {
	if p.tokenInfoURL != "" {
		return p.tokenInfoURL
	}
	return googleTokenInfoURL
}
