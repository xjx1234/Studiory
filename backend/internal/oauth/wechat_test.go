package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWechatProvider_Name(t *testing.T) {
	p := NewWechatProvider(WechatConfig{})
	if p.Name() != ProviderWechat {
		t.Fatalf("Name() = %q, want %q", p.Name(), ProviderWechat)
	}
}

func TestWechatProvider_DefaultUserInfoEndpoint(t *testing.T) {
	p := NewWechatProvider(WechatConfig{})
	if got := p.userInfoEndpoint(); got != wechatUserInfoURL {
		t.Fatalf("userInfoEndpoint() = %q, want %q", got, wechatUserInfoURL)
	}
}

func TestWechatProvider_MissingAccessTokenOrOpenIDReturnsInvalidToken(t *testing.T) {
	p := NewWechatProvider(WechatConfig{})

	cases := []VerifyRequest{
		{},
		{AccessToken: "only-token"},
		{OpenID: "only-openid"},
	}
	for _, req := range cases {
		if _, err := p.Verify(context.Background(), req); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("req=%+v: expected ErrInvalidToken, got %v", req, err)
		}
	}
}

func TestWechatProvider_ErrCodeReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40001, "errmsg": "invalid credential"})
	}))
	defer srv.Close()

	p := NewWechatProvider(WechatConfig{})
	p.userInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{AccessToken: "t", OpenID: "o"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWechatProvider_OpenIDMismatchReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回的 openid 与请求方传入的不一致（可能是伪造的 access_token 配不同 openid）。
		_ = json.NewEncoder(w).Encode(map[string]any{"openid": "someone-else"})
	}))
	defer srv.Close()

	p := NewWechatProvider(WechatConfig{})
	p.userInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{AccessToken: "t", OpenID: "expected-openid"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWechatProvider_EmptyOpenIDInResponseReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"openid": ""})
	}))
	defer srv.Close()

	p := NewWechatProvider(WechatConfig{})
	p.userInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{AccessToken: "t", OpenID: "expected-openid"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWechatProvider_MalformedJSONReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewWechatProvider(WechatConfig{})
	p.userInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{AccessToken: "t", OpenID: "o"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
