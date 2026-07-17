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

// mockProvider 用于覆盖 Router 里跟具体平台校验逻辑无关、只关心 Router 自身行为的分支。
type mockProvider struct {
	name     string
	identity *Identity
	err      error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Verify(_ context.Context, _ VerifyRequest) (*Identity, error) {
	return m.identity, m.err
}

func TestRouter_DevModeIgnoredWhenTokenPresent(t *testing.T) {
	// devMode=true，但请求带了 access_token：不能走“仅凭 open_id 免校验”的捷径，
	// 必须真正过一遍 Provider.Verify（否则会绕过生产场景下的真实校验）。
	called := false
	mock := &mockProvider{name: ProviderWechat, identity: &Identity{OpenID: "real-openid"}}
	r := NewRouter(nil, true, &countingProvider{Provider: mock, onCall: func() { called = true }})

	identity, err := r.Verify(context.Background(), VerifyRequest{
		Provider:    ProviderWechat,
		OpenID:      "wx_dev_openid",
		AccessToken: "real-token",
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !called {
		t.Fatal("expected Provider.Verify to be called when access_token is present, even in dev mode")
	}
	if identity.OpenID != "real-openid" {
		t.Fatalf("OpenID = %q, want real-openid", identity.OpenID)
	}
}

type countingProvider struct {
	Provider
	onCall func()
}

func (c *countingProvider) Verify(ctx context.Context, req VerifyRequest) (*Identity, error) {
	c.onCall()
	return c.Provider.Verify(ctx, req)
}

func TestRouter_EmptyIdentityOpenIDReturnsInvalidToken(t *testing.T) {
	mock := &mockProvider{name: ProviderGoogle, identity: &Identity{OpenID: "  "}}
	r := NewRouter(nil, false, mock)

	_, err := r.Verify(context.Background(), VerifyRequest{Provider: ProviderGoogle, AccessToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for blank OpenID, got %v", err)
	}
}

func TestRouter_NilIdentityReturnsInvalidToken(t *testing.T) {
	mock := &mockProvider{name: ProviderGoogle, identity: nil}
	r := NewRouter(nil, false, mock)

	_, err := r.Verify(context.Background(), VerifyRequest{Provider: ProviderGoogle, AccessToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for nil identity, got %v", err)
	}
}

func TestRouter_TrimsWhitespaceFromReturnedOpenID(t *testing.T) {
	mock := &mockProvider{name: ProviderGoogle, identity: &Identity{OpenID: "  sub-1  "}}
	r := NewRouter(nil, false, mock)

	identity, err := r.Verify(context.Background(), VerifyRequest{Provider: ProviderGoogle, AccessToken: "t"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "sub-1" {
		t.Fatalf("OpenID = %q, want trimmed %q", identity.OpenID, "sub-1")
	}
}

func TestRouter_ProviderNameIsCaseAndWhitespaceInsensitive(t *testing.T) {
	mock := &mockProvider{name: ProviderGoogle, identity: &Identity{OpenID: "sub-1"}}
	r := NewRouter(nil, false, mock)

	identity, err := r.Verify(context.Background(), VerifyRequest{Provider: "  GOOGLE  ", AccessToken: "t"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "sub-1" {
		t.Fatalf("OpenID = %q", identity.OpenID)
	}
}

func TestRouter_EmptyProviderReturnsInvalidToken(t *testing.T) {
	r := NewRouter(nil, false)

	_, err := r.Verify(context.Background(), VerifyRequest{Provider: "  ", AccessToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for empty provider, got %v", err)
	}
}

func TestRouter_NilProvidersAreSkippedDuringRegistration(t *testing.T) {
	// 注册时传入 nil Provider 不应 panic，也不应被当作可用 provider。
	r := NewRouter(nil, false, nil)

	_, err := r.Verify(context.Background(), VerifyRequest{Provider: ProviderGoogle, AccessToken: "t"})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}
