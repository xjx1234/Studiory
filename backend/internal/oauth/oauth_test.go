package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_DevModeAcceptsOpenIDOnly(t *testing.T) {
	r := NewRouter(nil, true)

	identity, err := r.Verify(context.Background(), VerifyRequest{
		Provider: ProviderWechat,
		OpenID:   "wx_dev_openid",
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "wx_dev_openid" {
		t.Fatalf("openid = %q", identity.OpenID)
	}
}

func TestRouter_ProductionRejectsOpenIDOnly(t *testing.T) {
	r := NewRouter(nil, false, NewGoogleProvider(GoogleConfig{}))

	_, err := r.Verify(context.Background(), VerifyRequest{
		Provider: ProviderGoogle,
		OpenID:   "should_not_work",
	})
	if err == nil {
		t.Fatal("expected error without token in production mode")
	}
}

func TestRouter_NoProvider(t *testing.T) {
	r := NewRouter(nil, false)

	_, err := r.Verify(context.Background(), VerifyRequest{
		Provider:    ProviderGoogle,
		AccessToken: "token",
	})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

func TestGoogleProvider_VerifyIDToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id_token") != "good-token" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_token"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":   "google-sub-123",
			"aud":   "my-client-id",
			"email": "user@example.com",
			"name":  "Tester",
		})
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{ClientID: "my-client-id"})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	identity, err := p.Verify(context.Background(), VerifyRequest{IDToken: "good-token"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "google-sub-123" || identity.Email != "user@example.com" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestGoogleProvider_AudMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "google-sub-123",
			"aud": "other-client",
		})
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{ClientID: "expected-client"})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "token"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWechatProvider_VerifyUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "wx-token" || r.URL.Query().Get("openid") != "wx-openid" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40001, "errmsg": "invalid credential"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openid":     "wx-openid",
			"nickname":   "微信用户",
			"headimgurl": "https://example.com/a.png",
		})
	}))
	defer srv.Close()

	p := NewWechatProvider(WechatConfig{AppID: "wx-app"})
	p.userInfoURL = srv.URL
	p.client = srv.Client()

	identity, err := p.Verify(context.Background(), VerifyRequest{
		AccessToken: "wx-token",
		OpenID:      "wx-openid",
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "wx-openid" || identity.Nickname != "微信用户" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestAppleProvider_RequiresClientID(t *testing.T) {
	p := NewAppleProvider(AppleConfig{})
	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "dummy"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
