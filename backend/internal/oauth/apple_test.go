package oauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testAppleKid = "test-kid-1"

func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func base64URLBigInt(n *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(n.Bytes())
}

func base64URLExponent(e int) string {
	buf := big.NewInt(int64(e)).Bytes()
	return base64.RawURLEncoding.EncodeToString(buf)
}

func newAppleJWKSServer(t *testing.T, pub *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"keys": []map[string]string{
				{
					"kty": "RSA",
					"kid": kid,
					"use": "sig",
					"alg": "RS256",
					"n":   base64URLBigInt(pub.N),
					"e":   base64URLExponent(pub.E),
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func signAppleTestToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func defaultAppleClaims(aud, sub string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":   "https://appleid.apple.com",
		"aud":   aud,
		"sub":   sub,
		"email": "user@example.com",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
}

func TestAppleProvider_Name(t *testing.T) {
	p := NewAppleProvider(AppleConfig{})
	if p.Name() != ProviderApple {
		t.Fatalf("Name() = %q, want %q", p.Name(), ProviderApple)
	}
}

func TestAppleProvider_DefaultJWKSEndpoint(t *testing.T) {
	p := NewAppleProvider(AppleConfig{})
	if got := p.jwksEndpoint(); got != appleJWKSURL {
		t.Fatalf("jwksEndpoint() = %q, want %q", got, appleJWKSURL)
	}
}

func TestAppleProvider_MissingIDTokenReturnsInvalidToken(t *testing.T) {
	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	_, err := p.Verify(context.Background(), VerifyRequest{})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_VerifySuccess(t *testing.T) {
	key := generateTestRSAKey(t)
	srv := newAppleJWKSServer(t, &key.PublicKey, testAppleKid)
	defer srv.Close()

	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, key, testAppleKid, defaultAppleClaims("client-1", "apple-sub-1"))

	identity, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.OpenID != "apple-sub-1" || identity.Email != "user@example.com" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestAppleProvider_CachesJWKSAcrossCalls(t *testing.T) {
	key := generateTestRSAKey(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		resp := map[string]any{
			"keys": []map[string]string{
				{
					"kty": "RSA", "kid": testAppleKid, "use": "sig", "alg": "RS256",
					"n": base64URLBigInt(key.N), "e": base64URLExponent(key.E),
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	for i := 0; i < 3; i++ {
		tokenStr := signAppleTestToken(t, key, testAppleKid, defaultAppleClaims("client-1", "apple-sub-1"))
		if _, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr}); err != nil {
			t.Fatalf("Verify #%d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Errorf("expected JWKS to be fetched once and cached, got %d fetches", hits)
	}
}

func TestAppleProvider_AudMismatchReturnsInvalidToken(t *testing.T) {
	key := generateTestRSAKey(t)
	srv := newAppleJWKSServer(t, &key.PublicKey, testAppleKid)
	defer srv.Close()

	p := NewAppleProvider(AppleConfig{ClientID: "expected-client"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, key, testAppleKid, defaultAppleClaims("other-client", "apple-sub-1"))

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_MissingSubReturnsInvalidToken(t *testing.T) {
	key := generateTestRSAKey(t)
	srv := newAppleJWKSServer(t, &key.PublicKey, testAppleKid)
	defer srv.Close()

	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	claims := defaultAppleClaims("client-1", "")
	delete(claims, "sub")
	tokenStr := signAppleTestToken(t, key, testAppleKid, claims)

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_UnexpectedSigningMethodRejected(t *testing.T) {
	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})

	// HS256 而非 RS256：Apple 只用 RS256 签发，收到别的算法应直接拒绝，不去尝试联网校验。
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, defaultAppleClaims("client-1", "sub-1"))
	tokenStr, err := token.SignedString([]byte("some-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_MissingKidRejected(t *testing.T) {
	key := generateTestRSAKey(t)
	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, defaultAppleClaims("client-1", "sub-1"))
	// 不设置 kid header。
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_UnknownKidRejected(t *testing.T) {
	key := generateTestRSAKey(t)
	srv := newAppleJWKSServer(t, &key.PublicKey, testAppleKid)
	defer srv.Close()

	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, key, "some-other-kid", defaultAppleClaims("client-1", "sub-1"))

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_JWKSFetchFailureRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	key := generateTestRSAKey(t)
	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, key, testAppleKid, defaultAppleClaims("client-1", "sub-1"))

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_MalformedJWKSRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	key := generateTestRSAKey(t)
	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, key, testAppleKid, defaultAppleClaims("client-1", "sub-1"))

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_NoUsableKeysRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// kty 不是 RSA，refreshJWKS 会跳过它，最终 keys 为空。
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{"kty": "EC", "kid": testAppleKid}},
		})
	}))
	defer srv.Close()

	key := generateTestRSAKey(t)
	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, key, testAppleKid, defaultAppleClaims("client-1", "sub-1"))

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAppleProvider_WrongSigningKeyRejected(t *testing.T) {
	signingKey := generateTestRSAKey(t)
	otherKey := generateTestRSAKey(t)
	// JWKS 里发布的是另一把 key，和签名用的 key 不匹配 —— 验签必须失败。
	srv := newAppleJWKSServer(t, &otherKey.PublicKey, testAppleKid)
	defer srv.Close()

	p := NewAppleProvider(AppleConfig{ClientID: "client-1"})
	p.jwksURL = srv.URL
	p.client = srv.Client()

	tokenStr := signAppleTestToken(t, signingKey, testAppleKid, defaultAppleClaims("client-1", "sub-1"))

	_, err := p.Verify(context.Background(), VerifyRequest{IDToken: tokenStr})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
