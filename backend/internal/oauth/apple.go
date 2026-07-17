package oauth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const appleJWKSURL = "https://appleid.apple.com/auth/keys"

// AppleConfig Sign in with Apple 配置。
type AppleConfig struct {
	ClientID string // 应用的 Services ID / Bundle ID，用于校验 id_token 的 aud
}

// AppleProvider 通过 Apple JWKS 校验 id_token 并解析 sub 作为 open_id。
type AppleProvider struct {
	cfg     AppleConfig
	client  *http.Client
	jwksURL string // 测试用覆盖；为空则使用默认生产地址

	jwksMu sync.RWMutex
	jwks   map[string]*rsa.PublicKey
}

func NewAppleProvider(cfg AppleConfig) *AppleProvider {
	return &AppleProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		jwks: make(map[string]*rsa.PublicKey),
	}
}

func (p *AppleProvider) Name() string { return ProviderApple }

func (p *AppleProvider) Verify(ctx context.Context, req VerifyRequest) (*Identity, error) {
	idToken := strings.TrimSpace(req.IDToken)
	if idToken == "" {
		return nil, fmt.Errorf("%w: apple requires id_token", ErrInvalidToken)
	}

	clientID := strings.TrimSpace(p.cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("%w: apple client_id", ErrNotConfigured)
	}

	token, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing kid in token header")
		}
		return p.publicKey(ctx, kid)
	}, jwt.WithIssuer("https://appleid.apple.com"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	aud, _ := claims["aud"].(string)
	if aud != clientID {
		return nil, fmt.Errorf("%w: apple aud mismatch", ErrInvalidToken)
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, ErrInvalidToken
	}

	email, _ := claims["email"].(string)
	return &Identity{
		OpenID: sub,
		Email:  email,
	}, nil
}

func (p *AppleProvider) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.jwksMu.RLock()
	key, ok := p.jwks[kid]
	p.jwksMu.RUnlock()
	if ok {
		return key, nil
	}

	if err := p.refreshJWKS(ctx); err != nil {
		return nil, err
	}

	p.jwksMu.RLock()
	defer p.jwksMu.RUnlock()
	key, ok = p.jwks[kid]
	if !ok {
		return nil, fmt.Errorf("kid %s not found in apple jwks", kid)
	}
	return key, nil
}

func (p *AppleProvider) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.jwksEndpoint(), nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Use string `json:"use"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return err
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaPublicKeyFromModExp(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return fmt.Errorf("no usable keys in apple jwks")
	}

	p.jwksMu.Lock()
	p.jwks = keys
	p.jwksMu.Unlock()
	return nil
}

func (p *AppleProvider) jwksEndpoint() string {
	if p.jwksURL != "" {
		return p.jwksURL
	}
	return appleJWKSURL
}

func rsaPublicKeyFromModExp(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}

	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		e = 65537
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}
