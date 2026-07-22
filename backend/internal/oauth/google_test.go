package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGoogleProvider_Name(t *testing.T) {
	p := NewGoogleProvider(GoogleConfig{})
	if p.Name() != ProviderGoogle {
		t.Fatalf("Name() = %q, want %q", p.Name(), ProviderGoogle)
	}
}

func TestGoogleProvider_DefaultTokenInfoEndpoint(t *testing.T) {
	p := NewGoogleProvider(GoogleConfig{})
	if got := p.tokenInfoEndpoint(); got != googleTokenInfoURL {
		t.Fatalf("tokenInfoEndpoint() = %q, want %q", got, googleTokenInfoURL)
	}
}

func TestGoogleProvider_EmptyTokenReturnsInvalidToken(t *testing.T) {
	p := NewGoogleProvider(GoogleConfig{})
	_, err := p.Verify(context.Background(), VerifyRequest{})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGoogleProvider_AccessTokenFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 没传 id_token 时应该改用 access_token 参数。
		if r.URL.Query().Get("access_token") != "at-good" || r.URL.Query().Get("id_token") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "sub-1", "aud": "test-client"})
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{ClientID: "test-client"})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	identity, err := p.Verify(context.Background(), VerifyRequest{AccessToken: "at-good"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "sub-1" {
		t.Fatalf("OpenID = %q, want sub-1", identity.OpenID)
	}
}

func TestGoogleProvider_NonOKStatusReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGoogleProvider_ErrorFieldInPayloadReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_token",
			"error_description": "Invalid Value",
		})
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGoogleProvider_MissingSubReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"aud": "any"})
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGoogleProvider_MalformedJSONReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGoogleProvider_NoClientIDReturnsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "sub-1", "aud": "whatever"})
	}))
	defer srv.Close()

	// ClientID 未配置：必须拒绝，不能跳过 aud 校验。
	p := NewGoogleProvider(GoogleConfig{})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestGoogleProvider_MissingAudReturnsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "sub-1"}) // 无 aud 字段
	}))
	defer srv.Close()

	p := NewGoogleProvider(GoogleConfig{ClientID: "test-client"})
	p.tokenInfoURL = srv.URL
	p.client = srv.Client()

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for missing aud, got %v", err)
	}
}

func TestGoogleProvider_TransportErrorPropagates(t *testing.T) {
	p := NewGoogleProvider(GoogleConfig{})
	p.tokenInfoURL = "http://127.0.0.1:1/unreachable" // 立即拒绝连接的端口
	p.client = &http.Client{Timeout: 2 * time.Second}

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: "t"})
	if err == nil {
		t.Fatal("expected a transport error")
	}
}
